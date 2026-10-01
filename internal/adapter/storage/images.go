// Package storage provides ImageStore adapters that load image bytes so the
// API can stream them without exposing storage paths (BR-14).
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/port"
)

// maxImageBytes bounds what the API will buffer for one image.
const maxImageBytes = 10 << 20

// validPath rejects anything that could escape the storage root.
func validPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func imageContentType(header string, data []byte) (string, error) {
	ct := header
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	if !strings.HasPrefix(ct, "image/") {
		ct = http.DetectContentType(data)
	}
	if !strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("storage: object is not an image (%s)", ct)
	}
	return ct, nil
}

// LocalStore reads images from a directory (development / offline use).
type LocalStore struct{ dir string }

func NewLocalStore(dir string) *LocalStore { return &LocalStore{dir: dir} }

func (s *LocalStore) Get(_ context.Context, p string) (*applicationport.Image, error) {
	if !validPath(p) {
		return nil, port.ErrNotFound
	}
	full := filepath.Join(s.dir, filepath.FromSlash(path.Clean(p)))
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, port.ErrNotFound
		}
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("storage: image too large")
	}
	ct, err := imageContentType("", data)
	if err != nil {
		return nil, err
	}
	return &applicationport.Image{Data: data, ContentType: ct}, nil
}

func (s *LocalStore) PreviewURL(string) string { return "" }

// SupabaseStore reads images from a Supabase Storage public bucket. The public
// URL is used only server-side; it is never sent to non-admin clients.
type SupabaseStore struct {
	baseURL string
	bucket  string
	client  *http.Client
}

func NewSupabaseStore(baseURL, bucket string) *SupabaseStore {
	return &SupabaseStore{
		baseURL: strings.TrimRight(baseURL, "/"), bucket: bucket,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SupabaseStore) objectURL(p string) string {
	segs := strings.Split(p, "/")
	for i := range segs {
		segs[i] = url.PathEscape(segs[i])
	}
	return s.baseURL + "/storage/v1/object/public/" + url.PathEscape(s.bucket) + "/" + strings.Join(segs, "/")
}

func (s *SupabaseStore) Get(ctx context.Context, p string) (*applicationport.Image, error) {
	if !validPath(p) {
		return nil, port.ErrNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objectURL(p), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("storage: upstream request failed")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest:
		return nil, port.ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("storage: upstream status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("storage: image too large")
	}
	ct, err := imageContentType(resp.Header.Get("Content-Type"), data)
	if err != nil {
		return nil, err
	}
	return &applicationport.Image{Data: data, ContentType: ct}, nil
}

// PreviewURL is the public object URL, for the admin library only.
func (s *SupabaseStore) PreviewURL(p string) string {
	if !validPath(p) {
		return ""
	}
	return s.objectURL(p)
}

// Chain tries each store in order and falls through on ErrNotFound.
type Chain struct{ stores []applicationport.ImageStore }

func NewChain(stores ...applicationport.ImageStore) *Chain { return &Chain{stores: stores} }

func (c *Chain) Get(ctx context.Context, p string) (*applicationport.Image, error) {
	var last error = port.ErrNotFound
	for _, s := range c.stores {
		img, err := s.Get(ctx, p)
		if err == nil {
			return img, nil
		}
		last = err
		if !errors.Is(err, port.ErrNotFound) {
			return nil, err
		}
	}
	return nil, last
}

func (c *Chain) PreviewURL(p string) string {
	for _, s := range c.stores {
		if u := s.PreviewURL(p); u != "" {
			return u
		}
	}
	return ""
}
