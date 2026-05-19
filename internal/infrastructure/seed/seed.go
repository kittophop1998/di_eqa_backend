package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

func Run(db *mongo.Database, supabaseURL, supabaseBucket string) error {
	// ใช้ timeout นานขึ้นเพราะ seedCellImageAssets ต้อง insert รูปหลายพันไฟล์
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	hospitalsColl := db.Collection("hospitals")
	usersColl := db.Collection("users")
	quizzesColl := db.Collection("quizzes")
	submissionsColl := db.Collection("submissions")
	cellImageAssetsColl := db.Collection("cell_image_assets")

	_, _ = hospitalsColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "code", Value: 1}},
	})
	_, _ = usersColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "hospitalId", Value: 1}, {Key: "username", Value: 1}},
	})

	// ── seed cell_image_assets จาก public/types (idempotent) ─────────────────
	// ถ้า folder ว่างเปล่า (เช่น บน production server ที่ไม่มีรูปใน git)
	// จะ fallback ไปดึง list รูปจาก Supabase Storage แทน
	publicTypesDir := filepath.Join(".", "public", "types")
	if err := seedCellImageAssets(ctx, cellImageAssetsColl, publicTypesDir, supabaseURL, supabaseBucket); err != nil {
		log.Printf("⚠️  seedCellImageAssets: %v", err)
	}
	// ─────────────────────────────────────────────────────────────────────────

	if err := seedHospitals(ctx, hospitalsColl); err != nil {
		return err
	}
	if err := seedUsers(ctx, usersColl, hospitalsColl); err != nil {
		return err
	}
	if err := seedQuizzes(ctx, quizzesColl, submissionsColl, usersColl, cellImageAssetsColl, supabaseURL, supabaseBucket); err != nil {
		return err
	}

	// ── ลบ ONE-TIME block เก่าออก ─────────────────────────────────────────────
	return nil
}

func seedHospitals(ctx context.Context, coll *mongo.Collection) error {
	hospitals := []entity.Hospital{
		{Code: "HOSP001", Name: "โรงพยาบาลศิริราช", Province: "กรุงเทพมหานคร", Logo: "🏥"},
		{Code: "HOSP002", Name: "โรงพยาบาลจุฬาลงกรณ์ สภากาชาดไทย", Province: "กรุงเทพมหานคร", Logo: "🏥"},
		{Code: "HOSP003", Name: "โรงพยาบาลรามาธิบดี", Province: "กรุงเทพมหานคร", Logo: "🏥"},
		{Code: "HOSP004", Name: "โรงพยาบาลมหาราชนครเชียงใหม่", Province: "เชียงใหม่", Logo: "🏥"},
		{Code: "HOSP005", Name: "โรงพยาบาลสงขลานครินทร์", Province: "สงขลา", Logo: "🏥"},
		{Code: "HOSP006", Name: "โรงพยาบาลศรีนครินทร์ ขอนแก่น", Province: "ขอนแก่น", Logo: "🏥"},
	}
	for _, h := range hospitals {
		count, err := coll.CountDocuments(ctx, bson.M{"code": h.Code})
		if err != nil {
			return err
		}
		if count == 0 {
			h.CreatedAt = time.Now()
			if _, err := coll.InsertOne(ctx, h); err != nil {
				return err
			}
			log.Printf("🏥 seeded hospital: %s - %s", h.Code, h.Name)
		}
	}
	return nil
}

func seedUsers(ctx context.Context, usersColl, hospitalsColl *mongo.Collection) error {
	var siriraj entity.Hospital
	if err := hospitalsColl.FindOne(ctx, bson.M{"code": "HOSP001"}).Decode(&siriraj); err != nil {
		return err
	}

	// ── seed super_admin (ไม่ผูกกับ รพ. ใด) ─────────────────────────────────
	// ถ้า document เดิมมี role=admin ให้อัปเกรดเป็น super_admin ก่อน
	_, _ = usersColl.UpdateOne(ctx,
		bson.M{"username": "admin", "role": entity.RoleAdmin},
		bson.M{"$set": bson.M{"role": entity.RoleSuperAdmin}},
	)
	adminCount, err := usersColl.CountDocuments(ctx, bson.M{"username": "admin"})
	if err != nil {
		return err
	}
	if adminCount == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin1234"), bcrypt.DefaultCost)
		adminUser := entity.User{
			Username:  "admin",
			FullName:  "Super Administrator",
			Email:     "admin@di-eqa.local",
			Password:  string(hash),
			Role:      entity.RoleSuperAdmin,
			CreatedAt: time.Now(),
		}
		if _, err := usersColl.InsertOne(ctx, adminUser); err != nil {
			return err
		}
		log.Printf("👤 seeded user: admin (role=super_admin, no hospital)")
	}

	// ── seed instructor + trainees ผูกกับ HOSP001 ────────────────────────────
	hospitalUsers := []struct {
		Username string
		FullName string
		Password string
		Role     string
	}{
		{"trainer", "วิทยากรอบรม", "trainer1234", entity.RoleInstructor},
		{"trainee01", "ผู้เข้าอบรม คนที่ 1", "trainee1234", entity.RoleUser},
		{"trainee02", "ผู้เข้าอบรม คนที่ 2", "trainee1234", entity.RoleUser},
	}

	for _, u := range hospitalUsers {
		count, err := usersColl.CountDocuments(ctx, bson.M{"hospitalId": siriraj.ID, "username": u.Username})
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		user := entity.User{
			HospitalID: siriraj.ID,
			Username:   u.Username,
			FullName:   u.FullName,
			Email:      u.Username + "@siriraj.local",
			Password:   string(hash),
			Role:       u.Role,
			CreatedAt:  time.Now(),
		}
		if _, err := usersColl.InsertOne(ctx, user); err != nil {
			return err
		}
		log.Printf("👤 seeded user: %s/%s (role=%s)", siriraj.Code, u.Username, u.Role)
	}
	return nil
}

// seedQuizzes สร้างข้อสอบ DI EQA แบบ "Cell Classification" (100 เซลล์)
// ถ้าเจอข้อสอบสคีมาเก่า (ไม่มี field cells) จะลบทิ้งแล้วเริ่มใหม่
// รูปเซลล์ดึงจาก collection cell_image_assets เท่านั้น — ไม่มี SVG fallback
// ImageURL ไม่ถูกเก็บใน quiz document แต่จะสร้าง dynamic ตอน handler ตอบ request
func seedQuizzes(ctx context.Context, quizzesColl, submissionsColl, usersColl, assetsColl *mongo.Collection, supabaseURL, supabaseBucket string) error {
	// ลบข้อสอบเก่าที่ยังเป็น schema multiple-choice (ไม่มี field "cells")
	delRes, _ := quizzesColl.DeleteMany(ctx, bson.M{"cells": bson.M{"$exists": false}})
	if delRes != nil && delRes.DeletedCount > 0 {
		log.Printf("🧹 removed %d legacy multiple-choice quizzes", delRes.DeletedCount)
		// ลบ submissions เดิมด้วยเพราะเฉลย/answers structure ต่างกัน
		_, _ = submissionsColl.DeleteMany(ctx, bson.M{"answers.choiceId": bson.M{"$exists": true}})
	}

	count, err := quizzesColl.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	var trainer entity.User
	_ = usersColl.FindOne(ctx, bson.M{"username": "trainer"}).Decode(&trainer)

	// 4 ชนิดเซลล์เม็ดเลือดขาวที่พบบ่อย — สีของ chip จะใช้ฝั่ง UI
	categories := []entity.CellCategory{
		{Key: "neutrophil", Label: "Neutrophil", Color: "#7c3aed"},
		{Key: "lymphocyte", Label: "Lymphocyte", Color: "#2563eb"},
		{Key: "eosinophil", Label: "Eosinophil", Color: "#ea580c"},
		{Key: "monocyte", Label: "Monocyte", Color: "#0891b2"},
	}

	// distribution ใกล้เคียงเลือดปกติ: Neutrophil ~60%, Lymphocyte ~28%,
	// Monocyte ~7%, Eosinophil ~5% — รวม 100 เซลล์
	plan := []struct {
		kind  string
		count int
	}{
		{"neutrophil", 60},
		{"lymphocyte", 28},
		{"monocyte", 7},
		{"eosinophil", 5},
	}

	cells := make([]entity.CellImage, 0, 100)
	rng := rand.New(rand.NewSource(42))

	// ── ดึงรูปจาก cell_image_assets ────────────────────────────────────────
	// จัดกลุ่ม assets ตาม cellType แล้วสุ่มเลือกตาม plan
	type assetDoc struct {
		ID       primitive.ObjectID `bson:"_id"`
		CellType string             `bson:"cellType"`
		Path     string             `bson:"path"`
	}
	assetsByType := map[string][]assetDoc{}
	cur, err := assetsColl.Find(ctx, bson.M{})
	if err == nil {
		var allAssets []assetDoc
		_ = cur.All(ctx, &allAssets)
		for _, a := range allAssets {
			assetsByType[a.CellType] = append(assetsByType[a.CellType], a)
		}
	}

	if len(assetsByType) == 0 {
		log.Printf("⚠️  cell_image_assets ว่างเปล่า — ข้าม quiz seed (รัน seedCellImageAssets ก่อน แล้ว restart)")
		return nil
	}

	idx := 0
	for _, p := range plan {
		typeAssets := assetsByType[p.kind]
		if len(typeAssets) == 0 {
			log.Printf("⚠️  ไม่มี asset สำหรับ %s — ข้ามเซลล์ชนิดนี้ %d ใบ", p.kind, p.count)
			continue
		}
		// shuffle แล้ว cycle ถ้า asset มีน้อยกว่าจำนวนที่ต้องการ
		rng.Shuffle(len(typeAssets), func(i, j int) { typeAssets[i], typeAssets[j] = typeAssets[j], typeAssets[i] })

		for i := 0; i < p.count; i++ {
			idx++
			a := typeAssets[i%len(typeAssets)] // cycle ถ้าไม่พอ
			cells = append(cells, entity.CellImage{
				ID:          fmt.Sprintf("c%03d", idx),
				AssetID:     a.ID,
				Path:        a.Path, // เก็บแค่ relative path — handler จะสร้าง full URL เอง
				CorrectType: p.kind,
			})
		}
	}
	// shuffle เพื่อให้เซลล์แต่ละชนิดกระจายไม่เรียงเป็นกอง ๆ
	rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })

	q := entity.Quiz{
		Title:       "DI EQA — นับแยกชนิดเซลล์เม็ดเลือดขาว (WBC Differential 100 cells)",
		Description: "จิ้มที่รูปเซลล์ฝั่งซ้าย แล้วจิ้มชนิดเซลล์ฝั่งขวาเพื่อจำแนก จนครบทั้ง 100 เซลล์",
		Category:    "โลหิตวิทยา",
		Categories:  categories,
		Cells:       cells,
		PassPercent: 80,
		DurationSec: 900,
		CreatedBy:   trainer.ID,
		CreatedAt:   time.Now(),
	}
	if _, err := quizzesColl.InsertOne(ctx, q); err != nil {
		return err
	}
	log.Printf("🩸 seeded WBC differential quiz (%d cells)", len(cells))
	return nil
}

// =============================================================================
// seedCellImageAssets — อ่าน folder public/types แล้ว insert path ลง MongoDB
//
// - ถ้ามีรูปใน publicTypesDir (dev / Docker ที่ COPY รูปมาด้วย) → อ่านจาก filesystem
// - ถ้า folder ว่างเปล่า (production server ที่ git ไม่มีรูป เพราะ public/ gitignored)
//   → fallback ไปดึง list object จาก Supabase Storage REST API แทน
// path ที่เก็บใน MongoDB จะเป็น "{cellType}/{filename}" เช่น "neutrophil/BNE_100878.jpg"
// =============================================================================

// knownCellTypes คือชื่อ folder ที่รู้จัก ใช้ตอน parse path จาก Supabase
var knownCellTypes = []string{"neutrophil", "lymphocyte", "eosinophil", "monocyte", "basophil", "erythroblast"}

// seedCellImageAssets สแกน publicTypesDir แล้ว insert CellImageAsset เข้า collection "cell_image_assets"
// ถ้า local dir ว่างเปล่าจะ fallback ไปใช้ Supabase Storage API (supabaseURL + supabaseBucket)
func seedCellImageAssets(ctx context.Context, coll *mongo.Collection, publicTypesDir, supabaseURL, supabaseBucket string) error {
	total, skipped, err := seedFromLocalFS(ctx, coll, publicTypesDir)
	if err != nil {
		log.Printf("⚠️  seedCellImageAssets local: %v — trying Supabase fallback", err)
	}
	if total+skipped > 0 {
		// มีข้อมูลจาก local filesystem แล้ว ไม่ต้อง fallback
		log.Printf("🖼️  seeded cell_image_assets (local): inserted=%d, skipped=%d", total, skipped)
		return nil
	}

	// Fallback: ดึง list จาก Supabase Storage
	log.Printf("📡 local public/types ว่างเปล่า — fallback ไปดึงรายการรูปจาก Supabase Storage...")
	total, skipped, err = seedFromSupabase(ctx, coll, supabaseURL, supabaseBucket)
	if err != nil {
		return fmt.Errorf("seedCellImageAssets supabase fallback: %w", err)
	}
	log.Printf("🖼️  seeded cell_image_assets (supabase): inserted=%d, skipped=%d", total, skipped)
	return nil
}

// seedFromLocalFS อ่านรูปจาก filesystem แล้ว insert ลง MongoDB โดยใช้ InsertMany แบบ batch
// คืน (inserted, skipped, error)
func seedFromLocalFS(ctx context.Context, coll *mongo.Collection, publicTypesDir string) (int, int, error) {
	entries, err := os.ReadDir(publicTypesDir)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read dir %s: %w", publicTypesDir, err)
	}

	// ดึง paths ที่มีอยู่แล้วทั้งหมดมาเก็บใน set เดียว (ทำ 1 query แทนที่จะทำทีละไฟล์)
	existingPaths, err := fetchExistingPaths(ctx, coll)
	if err != nil {
		return 0, 0, err
	}

	skipped := 0
	var batch []interface{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cellType := entry.Name()
		cellDir := filepath.Join(publicTypesDir, cellType)

		files, err := os.ReadDir(cellDir)
		if err != nil {
			log.Printf("⚠️  cannot read %s: %v", cellDir, err)
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}

			path := cellType + "/" + name

			if existingPaths[path] {
				skipped++
				continue
			}

			batch = append(batch, entity.CellImageAsset{
				CellType:  cellType,
				Filename:  name,
				Path:      path,
				CreatedAt: time.Now(),
			})
		}
	}

	if len(batch) == 0 {
		return 0, skipped, nil
	}

	total, err := insertManyBatch(ctx, coll, batch)
	return total, skipped, err
}

// seedFromSupabase ดึง list object จาก Supabase Storage REST API แล้ว insert ลง MongoDB (batch)
// ใช้ตอนที่ local public/types ว่างเปล่า (production deployment ที่ images ไม่ได้อยู่ใน git)
func seedFromSupabase(ctx context.Context, coll *mongo.Collection, supabaseURL, bucket string) (int, int, error) {
	existingPaths, err := fetchExistingPaths(ctx, coll)
	if err != nil {
		return 0, 0, err
	}

	skipped := 0
	var batch []interface{}

	for _, cellType := range knownCellTypes {
		// Supabase Storage List API: POST /storage/v1/object/list/{bucket}
		listURL := fmt.Sprintf("%s/storage/v1/object/list/%s", strings.TrimRight(supabaseURL, "/"), bucket)
		body := fmt.Sprintf(`{"prefix":"%s/","limit":10000}`, cellType)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, listURL, strings.NewReader(body))
		if err != nil {
			log.Printf("⚠️  supabase list request %s: %v", cellType, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("⚠️  supabase list %s: %v", cellType, err)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("⚠️  supabase list %s: status %d", cellType, resp.StatusCode)
			continue
		}

		var items []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(respBody, &items); err != nil {
			log.Printf("⚠️  supabase list %s parse: %v", cellType, err)
			continue
		}

		for _, item := range items {
			name := item.Name
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}

			path := cellType + "/" + name

			if existingPaths[path] {
				skipped++
				continue
			}

			batch = append(batch, entity.CellImageAsset{
				CellType:  cellType,
				Filename:  name,
				Path:      path,
				CreatedAt: time.Now(),
			})
		}
	}

	if len(batch) == 0 {
		return 0, skipped, nil
	}

	total, err := insertManyBatch(ctx, coll, batch)
	return total, skipped, err
}

// fetchExistingPaths ดึง path ทั้งหมดที่มีอยู่ใน collection มาเก็บเป็น set
// เพื่อ check ซ้ำได้เร็วโดยไม่ต้อง query ทีละ document
func fetchExistingPaths(ctx context.Context, coll *mongo.Collection) (map[string]bool, error) {
	cur, err := coll.Find(ctx, bson.M{}, &options.FindOptions{
		Projection: bson.M{"path": 1, "_id": 0},
	})
	if err != nil {
		return nil, fmt.Errorf("fetchExistingPaths: %w", err)
	}
	defer cur.Close(ctx)

	set := map[string]bool{}
	for cur.Next(ctx) {
		var doc struct {
			Path string `bson:"path"`
		}
		if err := cur.Decode(&doc); err == nil && doc.Path != "" {
			set[doc.Path] = true
		}
	}
	return set, cur.Err()
}

// insertManyBatch insert slice ของ documents ทีละ 500 ชิ้นเพื่อป้องกัน payload ใหญ่เกิน
// คืนจำนวน documents ที่ insert สำเร็จ
func insertManyBatch(ctx context.Context, coll *mongo.Collection, docs []interface{}) (int, error) {
	const batchSize = 500
	total := 0
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		res, err := coll.InsertMany(ctx, docs[i:end])
		if err != nil {
			return total, fmt.Errorf("insertManyBatch: %w", err)
		}
		total += len(res.InsertedIDs)
	}
	return total, nil
}
