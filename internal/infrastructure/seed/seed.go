// Package seed loads development fixtures (hospitals, cell types and cell
// images). It is used only by cmd/seed, never on the server boot path. Every
// operation is an insert-if-missing upsert: it never deletes or overwrites
// existing documents.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Options selects the optional image sources.
type Options struct {
	// ImagesDir is scanned as <dir>/<typeKey>/<file>.(jpg|jpeg|png|webp). Empty skips it.
	ImagesDir string
	// SupabaseURL and SupabaseBucket, when both set, list images from Storage.
	SupabaseURL    string
	SupabaseBucket string
}

// Run seeds fixtures. It is safe to run repeatedly.
func Run(ctx context.Context, db *mongo.Database, opts Options) error {
	if err := seedHospitals(ctx, db.Collection("hospitals")); err != nil {
		return fmt.Errorf("hospitals: %w", err)
	}
	if err := seedCellTypes(ctx, db.Collection("cell_types")); err != nil {
		return fmt.Errorf("cell types: %w", err)
	}
	images := db.Collection("cell_images")
	if opts.ImagesDir != "" {
		n, err := seedImagesFromDir(ctx, images, opts.ImagesDir)
		if err != nil {
			return fmt.Errorf("images from %s: %w", opts.ImagesDir, err)
		}
		log.Printf("cell_images from directory: %d new", n)
	}
	if opts.SupabaseURL != "" && opts.SupabaseBucket != "" {
		n, err := seedImagesFromSupabase(ctx, images, opts.SupabaseURL, opts.SupabaseBucket)
		if err != nil {
			return fmt.Errorf("images from supabase: %w", err)
		}
		log.Printf("cell_images from Supabase: %d new", n)
	}
	return nil
}

func seedHospitals(ctx context.Context, coll *mongo.Collection) error {
	hospitals := []entity.Hospital{
		{Code: "HOSP001", Name: "โรงพยาบาลศิริราช", Logo: "🏥", SubDistrict: "ศิริราช", District: "บางกอกน้อย", Province: "กรุงเทพมหานคร", PostalCode: "10700"},
		{Code: "HOSP002", Name: "โรงพยาบาลจุฬาลงกรณ์ สภากาชาดไทย", Logo: "🏥", SubDistrict: "ปทุมวัน", District: "ปทุมวัน", Province: "กรุงเทพมหานคร", PostalCode: "10330"},
		{Code: "HOSP003", Name: "โรงพยาบาลรามาธิบดี", Logo: "🏥", SubDistrict: "ทุ่งพญาไท", District: "ราชเทวี", Province: "กรุงเทพมหานคร", PostalCode: "10400"},
		{Code: "HOSP004", Name: "โรงพยาบาลมหาราชนครเชียงใหม่", Logo: "🏥", SubDistrict: "ศรีภูมิ", District: "เมืองเชียงใหม่", Province: "เชียงใหม่", PostalCode: "50200"},
		{Code: "HOSP005", Name: "โรงพยาบาลสงขลานครินทร์", Logo: "🏥", SubDistrict: "คอหงส์", District: "หาดใหญ่", Province: "สงขลา", PostalCode: "90110"},
		{Code: "HOSP006", Name: "โรงพยาบาลศรีนครินทร์ ขอนแก่น", Logo: "🏥", SubDistrict: "ในเมือง", District: "เมืองขอนแก่น", Province: "ขอนแก่น", PostalCode: "40002"},
	}
	now := time.Now().UTC()
	for _, h := range hospitals {
		res, err := coll.UpdateOne(ctx, bson.M{"code": h.Code}, bson.M{"$setOnInsert": bson.M{
			"code": h.Code, "name": h.Name, "logo": h.Logo, "province": h.Province, "district": h.District,
			"subDistrict": h.SubDistrict, "postalCode": h.PostalCode, "active": true, "createdAt": now,
		}}, options.Update().SetUpsert(true))
		if err != nil {
			return err
		}
		if res.UpsertedCount > 0 {
			log.Printf("seeded hospital %s", h.Code)
		}
	}
	return nil
}

func seedCellTypes(ctx context.Context, coll *mongo.Collection) error {
	now := time.Now().UTC()
	for _, t := range entity.DefaultCellTypes() {
		if _, err := coll.UpdateOne(ctx, bson.M{"key": t.Key}, bson.M{"$setOnInsert": bson.M{
			"key": t.Key, "label": t.Label, "sortOrder": t.SortOrder, "active": true, "createdAt": now,
		}}, options.Update().SetUpsert(true)); err != nil {
			return err
		}
	}
	return nil
}

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

type imageRef struct{ typeKey, path string }

func seedImagesFromDir(ctx context.Context, coll *mongo.Collection, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var refs []imageRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, e.Name()))
		if err != nil {
			return 0, err
		}
		for _, f := range files {
			if f.IsDir() || !imageExts[strings.ToLower(filepath.Ext(f.Name()))] {
				continue
			}
			refs = append(refs, imageRef{typeKey: e.Name(), path: e.Name() + "/" + f.Name()})
		}
	}
	return upsertImages(ctx, coll, refs)
}

func seedImagesFromSupabase(ctx context.Context, coll *mongo.Collection, baseURL, bucket string) (int, error) {
	var refs []imageRef
	for _, t := range entity.DefaultCellTypes() {
		listURL := fmt.Sprintf("%s/storage/v1/object/list/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(bucket))
		body, _ := json.Marshal(map[string]any{"prefix": t.Key + "/", "limit": 10000})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, listURL, strings.NewReader(string(body)))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("supabase list %s: status %d (skipped)", t.Key, resp.StatusCode)
			continue
		}
		var items []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &items); err != nil {
			return 0, err
		}
		for _, it := range items {
			if imageExts[strings.ToLower(filepath.Ext(it.Name))] {
				refs = append(refs, imageRef{typeKey: t.Key, path: t.Key + "/" + it.Name})
			}
		}
	}
	return upsertImages(ctx, coll, refs)
}

// upsertImages inserts missing images in batches; existing paths are untouched.
func upsertImages(ctx context.Context, coll *mongo.Collection, refs []imageRef) (int, error) {
	now := time.Now().UTC()
	inserted := 0
	const batch = 500
	for i := 0; i < len(refs); i += batch {
		end := i + batch
		if end > len(refs) {
			end = len(refs)
		}
		models := make([]mongo.WriteModel, 0, end-i)
		for _, r := range refs[i:end] {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"path": r.path}).SetUpsert(true).
				SetUpdate(bson.M{"$setOnInsert": bson.M{"typeKey": r.typeKey, "path": r.path, "active": true, "createdAt": now}}))
		}
		res, err := coll.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
		if err != nil {
			return inserted, err
		}
		inserted += int(res.UpsertedCount)
	}
	return inserted, nil
}
