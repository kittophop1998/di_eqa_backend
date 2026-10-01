// Command seed loads development fixtures (hospitals, cell types, cell
// images). It refuses to run when APP_ENV=production. It only inserts missing
// documents; nothing is deleted or overwritten. Run cmd/migrate first.
//
//	APP_ENV=development MONGO_URI=... MONGO_DB=... go run ./cmd/seed -images ./public/types
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/infrastructure/db"
	"github.com/di-eqa/backend/internal/infrastructure/seed"
)

func main() {
	images := flag.String("images", "", "directory of <typeKey>/<file> images to register in cell_images")
	supabase := flag.Bool("supabase", false, "also list images from SUPABASE_URL/SUPABASE_BUCKET")
	flag.Parse()

	if env := strings.ToLower(os.Getenv("APP_ENV")); env == "production" || env == "prod" {
		log.Fatal("refusing to seed: APP_ENV=production")
	}
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI must be set explicitly (there is no default, to avoid seeding the wrong database)")
	}
	name := os.Getenv("MONGO_DB")
	if name == "" {
		name = "di_eqa"
	}
	conn, err := db.Connect(uri, name)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	opts := seed.Options{ImagesDir: *images}
	if *supabase {
		opts.SupabaseURL, opts.SupabaseBucket = os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_BUCKET")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := seed.Run(ctx, conn.DB, opts); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("seed finished (database %q)", name)
}
