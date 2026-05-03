package seed

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
)

// renderCellSVG วาดเซลล์เม็ดเลือดขาว 1 ใบเป็น SVG
// kind ∈ {"neutrophil","lymphocyte","monocyte","eosinophil"}
// seed ใช้เพื่อให้แต่ละ cell มีหน้าตาต่างกันเล็กน้อย (rotate, ตำแหน่ง RBC พื้นหลัง, granules)
func renderCellSVG(kind string, seed int64) string {
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder

	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">`)
	// พื้นสไลด์สี off-white อมชมพู
	b.WriteString(`<rect width="100" height="100" fill="#fff5f0"/>`)

	// RBC พื้นหลัง 6-9 ใบ — สีชมพูอ่อน รูโดนัทตรงกลาง
	rbcCount := 6 + r.Intn(4)
	for i := 0; i < rbcCount; i++ {
		cx := r.Intn(110) - 5
		cy := r.Intn(110) - 5
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="9" fill="#f6b8b0" opacity="0.55"/>`, cx, cy))
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="3.5" fill="#fff5f0" opacity="0.85"/>`, cx, cy))
	}

	// WBC อยู่ตรงกลาง — เลือกตำแหน่งใกล้กลางและหมุนตาม seed
	rot := r.Intn(360)
	b.WriteString(fmt.Sprintf(`<g transform="rotate(%d 50 50)">`, rot))
	switch kind {
	case "neutrophil":
		drawNeutrophil(&b, r)
	case "lymphocyte":
		drawLymphocyte(&b, r)
	case "monocyte":
		drawMonocyte(&b, r)
	case "eosinophil":
		drawEosinophil(&b, r)
	}
	b.WriteString(`</g>`)

	b.WriteString(`</svg>`)
	return b.String()
}

// neutrophil — segmented neutrophil: nucleus 3-5 lobes ต่อกันด้วย chromatin filament
// cytoplasm สีชมพูซีด มี granules ละเอียดสีม่วง
func drawNeutrophil(b *strings.Builder, r *rand.Rand) {
	b.WriteString(`<circle cx="50" cy="50" r="34" fill="#ffe1d6" stroke="#8a5a73" stroke-width="0.6"/>`)
	// nucleus segments
	b.WriteString(`<g fill="#3d1466">`)
	b.WriteString(`<ellipse cx="38" cy="42" rx="11" ry="9"/>`)
	b.WriteString(`<ellipse cx="60" cy="36" rx="10" ry="9"/>`)
	b.WriteString(`<ellipse cx="64" cy="58" rx="11" ry="9"/>`)
	b.WriteString(`<ellipse cx="40" cy="64" rx="10" ry="9"/>`)
	// connecting filaments
	b.WriteString(`<path d="M44 41 Q50 36 54 38" stroke="#3d1466" stroke-width="3" fill="none" stroke-linecap="round"/>`)
	b.WriteString(`<path d="M62 44 Q66 50 64 56" stroke="#3d1466" stroke-width="3" fill="none" stroke-linecap="round"/>`)
	b.WriteString(`<path d="M52 62 Q48 64 44 62" stroke="#3d1466" stroke-width="3" fill="none" stroke-linecap="round"/>`)
	b.WriteString(`<path d="M40 56 Q36 50 38 46" stroke="#3d1466" stroke-width="3" fill="none" stroke-linecap="round"/>`)
	b.WriteString(`</g>`)
	// fine granules
	for i := 0; i < 14; i++ {
		gx := 22 + r.Intn(56)
		gy := 22 + r.Intn(56)
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="0.7" fill="#a36ad6" opacity="0.55"/>`, gx, gy))
	}
}

// lymphocyte — เซลล์เล็ก nucleus ก้อนใหญ่กลม กินเนื้อที่เกือบหมด
// cytoplasm สีฟ้าใส ๆ บาง ๆ
func drawLymphocyte(b *strings.Builder, r *rand.Rand) {
	_ = r
	b.WriteString(`<circle cx="50" cy="50" r="28" fill="#cfdfff" stroke="#5a6db1" stroke-width="0.6"/>`)
	b.WriteString(`<circle cx="50" cy="49" r="22" fill="#321966"/>`)
	// chromatin texture (เส้นทึบเล็ก ๆ ใน nucleus)
	b.WriteString(`<circle cx="44" cy="44" r="3" fill="#1a0833" opacity="0.6"/>`)
	b.WriteString(`<circle cx="56" cy="46" r="3" fill="#1a0833" opacity="0.6"/>`)
	b.WriteString(`<circle cx="52" cy="56" r="3" fill="#1a0833" opacity="0.6"/>`)
	b.WriteString(`<circle cx="46" cy="54" r="2" fill="#1a0833" opacity="0.6"/>`)
}

// monocyte — เซลล์ใหญ่ที่สุด nucleus รูปไต/ตัว C cytoplasm สีเทาอมฟ้ามาก
// อาจมี vacuoles (ฟอง) เล็ก ๆ
func drawMonocyte(b *strings.Builder, r *rand.Rand) {
	b.WriteString(`<circle cx="50" cy="50" r="38" fill="#c9d8e3" stroke="#4f6878" stroke-width="0.6"/>`)
	// kidney-shaped nucleus — วาดด้วย path
	b.WriteString(`<path d="M30 38 Q22 50 30 64 Q44 72 60 66 Q60 56 52 52 Q60 48 60 38 Q44 30 30 38 Z" fill="#3a1a66"/>`)
	// vacuoles
	for i := 0; i < 3; i++ {
		gx := 55 + r.Intn(20)
		gy := 30 + r.Intn(40)
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="2" fill="#fff5f0" opacity="0.85"/>`, gx, gy))
	}
}

// eosinophil — bi-lobed nucleus + granules สีส้มแดงเด่น เม็ดใหญ่กว่า neutrophil
func drawEosinophil(b *strings.Builder, r *rand.Rand) {
	b.WriteString(`<circle cx="50" cy="50" r="33" fill="#ffd9c4" stroke="#8a5a40" stroke-width="0.6"/>`)
	// 2 lobes ของ nucleus
	b.WriteString(`<g fill="#3d1466">`)
	b.WriteString(`<ellipse cx="36" cy="50" rx="13" ry="11"/>`)
	b.WriteString(`<ellipse cx="64" cy="50" rx="13" ry="11"/>`)
	b.WriteString(`<rect x="44" y="46" width="12" height="8" rx="3"/>`)
	b.WriteString(`</g>`)
	// orange-red granules (ใหญ่/หนาแน่น)
	for i := 0; i < 22; i++ {
		gx := 22 + r.Intn(56)
		gy := 22 + r.Intn(56)
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="2" fill="#ff6b35" opacity="0.85"/>`, gx, gy))
	}
}

func svgToDataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
