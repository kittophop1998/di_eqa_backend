// Package config loads and validates process configuration from the
// environment. There are no default secrets: production refuses to start
// without a strong JWT_SECRET (A-01, BR-50).
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// MinJWTSecretBytes is the minimum JWT_SECRET length in production.
const MinJWTSecretBytes = 32

// revokedSecretHashes are SHA-256 hashes of secrets that were committed to the
// repository in the past. They are treated as compromised and rejected in every
// environment. (Only hashes are stored, so the secrets are not repeated here.)
var revokedSecretHashes = map[string]bool{
	"8f7aec8dbbf9907e20e40d449c7a5ba3291c41dec9ede92de8c314b138f0416a": true,
	"9a6f80d0660dd30f32a528204e9c7acd1561d359ec616a78709b0fd1dd97b04d": true,
}

// Config is the validated process configuration.
type Config struct {
	Env        string
	Production bool

	Port          string
	MongoURI      string
	MongoDB       string
	RedisAddr     string
	RedisPassword string

	JWTSecret string
	JWTExpiry time.Duration

	AllowedOrigins []string

	SupabaseURL    string
	SupabaseBucket string
	ImageLocalDir  string

	AdminInitialUsername string
	AdminInitialPassword string

	RunMigrations bool

	// Warnings are non-fatal findings the caller should log.
	Warnings []string
}

// Load reads the real process environment.
func Load() (*Config, error) { return LoadFrom(os.LookupEnv) }

// LoadFrom reads configuration through lookup so it can be tested without
// touching the process environment.
func LoadFrom(lookup func(string) (string, bool)) (*Config, error) {
	get := func(key string) string {
		v, _ := lookup(key)
		return strings.TrimSpace(v)
	}
	c := &Config{Env: strings.ToLower(get("APP_ENV"))}
	c.Production = c.Env == "production" || c.Env == "prod"
	if c.Env == "" {
		c.Env = "development"
	}
	var problems []string

	c.Port = orDefault(get("PORT"), "8080")

	// JWT secret
	c.JWTSecret = get("JWT_SECRET")
	if revokedSecretHashes[sha256Hex(c.JWTSecret)] {
		problems = append(problems, "JWT_SECRET is a previously leaked secret; generate a new one")
	}
	switch {
	case c.Production && len(c.JWTSecret) < MinJWTSecretBytes:
		problems = append(problems, fmt.Sprintf("JWT_SECRET is required in production and must be at least %d bytes", MinJWTSecretBytes))
	case c.JWTSecret == "":
		secret, err := randomSecret()
		if err != nil {
			return nil, err
		}
		c.JWTSecret = secret
		c.Warnings = append(c.Warnings, "JWT_SECRET is not set: using a random per-process secret (tokens are invalidated on restart)")
	}
	ttl := 12
	if v := get("JWT_TTL_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 24*30 {
			problems = append(problems, "JWT_TTL_HOURS must be an integer between 1 and 720")
		} else {
			ttl = n
		}
	}
	c.JWTExpiry = time.Duration(ttl) * time.Hour

	// Datastores
	c.MongoURI = get("MONGO_URI")
	c.MongoDB = orDefault(get("MONGO_DB"), "di_eqa")
	c.RedisAddr = firstNonEmpty(get("REDIS_URL"), get("REDIS_ADDR"))
	c.RedisPassword = get("REDIS_PASSWORD")
	if c.Production {
		if c.MongoURI == "" {
			problems = append(problems, "MONGO_URI is required in production")
		}
		if c.RedisAddr == "" {
			problems = append(problems, "REDIS_URL (or REDIS_ADDR) is required in production")
		}
	} else {
		c.MongoURI = orDefault(c.MongoURI, "mongodb://localhost:27017")
		c.RedisAddr = orDefault(c.RedisAddr, "redis://localhost:6379")
	}

	// CORS: exact origins only, never "*".
	origins := get("ALLOWED_ORIGINS")
	if origins == "" {
		origins = get("ALLOWED_ORIGIN") // legacy name
	}
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if o == "*" || err != nil || u.Scheme == "" || u.Host == "" {
			problems = append(problems, fmt.Sprintf("ALLOWED_ORIGINS entry %q must be an exact origin such as https://app.example.com", o))
			continue
		}
		c.AllowedOrigins = append(c.AllowedOrigins, o)
	}
	if len(c.AllowedOrigins) == 0 {
		c.Warnings = append(c.Warnings, "ALLOWED_ORIGINS is empty: no CORS headers will be sent (same-origin only)")
	}

	// Image source (no default project URL)
	c.SupabaseURL = get("SUPABASE_URL")
	c.SupabaseBucket = get("SUPABASE_BUCKET")
	c.ImageLocalDir = get("IMAGE_LOCAL_DIR")
	if (c.SupabaseURL == "") != (c.SupabaseBucket == "") {
		problems = append(problems, "SUPABASE_URL and SUPABASE_BUCKET must be set together")
	}
	if c.Production && c.SupabaseURL == "" && c.ImageLocalDir == "" {
		problems = append(problems, "an image source is required in production: set SUPABASE_URL+SUPABASE_BUCKET or IMAGE_LOCAL_DIR")
	}

	c.AdminInitialUsername = get("ADMIN_INITIAL_USERNAME")
	c.AdminInitialPassword = get("ADMIN_INITIAL_PASSWORD")
	if (c.AdminInitialUsername == "") != (c.AdminInitialPassword == "") {
		problems = append(problems, "ADMIN_INITIAL_USERNAME and ADMIN_INITIAL_PASSWORD must be set together")
	}

	if v := get("RUN_MIGRATIONS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			problems = append(problems, "RUN_MIGRATIONS must be true or false")
		}
		c.RunMigrations = b
	}

	if len(problems) > 0 {
		return nil, errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomSecret() (string, error) {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
