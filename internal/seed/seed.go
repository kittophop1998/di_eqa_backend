package seed

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

func Run(db *mongo.Database, supabaseURL, supabaseBucket string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	// publicTypesDir := filepath.Join(".", "public", "types")
	// if err := seedCellImageAssets(ctx, cellImageAssetsColl, publicTypesDir); err != nil {
	// 	log.Printf("⚠️  seedCellImageAssets: %v", err)
	// }
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
	hospitals := []models.Hospital{
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
	var siriraj models.Hospital
	if err := hospitalsColl.FindOne(ctx, bson.M{"code": "HOSP001"}).Decode(&siriraj); err != nil {
		return err
	}

	defaultUsers := []struct {
		Username string
		FullName string
		Password string
		Role     string
	}{
		{"admin", "ผู้ดูแลระบบ", "admin1234", models.RoleAdmin},
		{"trainer", "วิทยากรอบรม", "trainer1234", models.RoleInstructor},
		{"trainee01", "ผู้เข้าอบรม คนที่ 1", "trainee1234", models.RoleUser},
		{"trainee02", "ผู้เข้าอบรม คนที่ 2", "trainee1234", models.RoleUser},
	}

	for _, u := range defaultUsers {
		count, err := usersColl.CountDocuments(ctx, bson.M{"hospitalId": siriraj.ID, "username": u.Username})
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		user := models.User{
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

	var trainer models.User
	_ = usersColl.FindOne(ctx, bson.M{"username": "trainer"}).Decode(&trainer)

	// 4 ชนิดเซลล์เม็ดเลือดขาวที่พบบ่อย — สีของ chip จะใช้ฝั่ง UI
	categories := []models.CellCategory{
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

	cells := make([]models.CellImage, 0, 100)
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
			cells = append(cells, models.CellImage{
				ID:          fmt.Sprintf("c%03d", idx),
				AssetID:     a.ID,
				Path:        a.Path, // เก็บแค่ relative path — handler จะสร้าง full URL เอง
				CorrectType: p.kind,
			})
		}
	}
	// shuffle เพื่อให้เซลล์แต่ละชนิดกระจายไม่เรียงเป็นกอง ๆ
	rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })

	q := models.Quiz{
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
// ใช้ครั้งเดียว: เปิด comment ที่ Run() แล้ว restart server จากนั้น comment กลับ
// path บน Supabase จะเป็น "{cellType}/{filename}" เช่น "neutrophil/BNE_100878.jpg"
// =============================================================================

// seedCellImageAssets สแกน publicTypesDir แล้ว insert CellImageAsset เข้า collection "cell_image_assets"
// publicTypesDir ควรเป็น absolute path ไปยัง backend/public/types
func seedCellImageAssets(ctx context.Context, coll *mongo.Collection, publicTypesDir string) error {
	entries, err := os.ReadDir(publicTypesDir)
	if err != nil {
		return fmt.Errorf("seedCellImageAssets: cannot read dir %s: %w", publicTypesDir, err)
	}

	total := 0
	skipped := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue // ข้าม .DS_Store หรือไฟล์อื่น ๆ ที่ไม่ใช่ folder
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
			// กรอกเฉพาะไฟล์รูป (.jpg, .jpeg, .png)
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}

			path := cellType + "/" + name // path บน Supabase bucket

			// ข้ามถ้ามีอยู่แล้ว (idempotent)
			count, _ := coll.CountDocuments(ctx, bson.M{"path": path})
			if count > 0 {
				skipped++
				continue
			}

			asset := models.CellImageAsset{
				CellType:  cellType,
				Filename:  name,
				Path:      path,
				CreatedAt: time.Now(),
			}
			if _, err := coll.InsertOne(ctx, asset); err != nil {
				return fmt.Errorf("seedCellImageAssets: insert %s: %w", path, err)
			}
			total++
		}
	}

	log.Printf("🖼️  seeded cell_image_assets: inserted=%d, skipped(already exists)=%d", total, skipped)
	return nil
}
