package service

import (
	"context"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
)

// CellLibraryService implements the read side of the cell library (C-05).
type CellLibraryService struct {
	types  port.CellTypeRepository
	images port.CellImageRepository
	store  applicationport.ImageStore
}

func NewCellLibraryService(t port.CellTypeRepository, i port.CellImageRepository, store applicationport.ImageStore) *CellLibraryService {
	return &CellLibraryService{types: t, images: i, store: store}
}

// AdminCellType is the admin view of a cell type (§5.6).
type AdminCellType struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	SortOrder        int    `json:"sortOrder"`
	Active           bool   `json:"active"`
	ImageCount       int    `json:"imageCount"`
	ActiveImageCount int    `json:"activeImageCount"`
}

// CellTypesOutput is GET /admin/cell-types.
type CellTypesOutput struct {
	Items          []AdminCellType `json:"items"`
	ActivePoolSize int             `json:"activePoolSize"`
}

func (s *CellLibraryService) ListTypes(ctx context.Context) (*CellTypesOutput, error) {
	types, err := s.types.ListAll(ctx)
	if err != nil {
		return nil, internal(err)
	}
	counts, err := s.images.CountByType(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := &CellTypesOutput{Items: make([]AdminCellType, 0, len(types))}
	for _, t := range types {
		c := counts[t.Key]
		out.Items = append(out.Items, AdminCellType{
			Key: t.Key, Label: t.Label, SortOrder: t.SortOrder, Active: t.Active,
			ImageCount: c.Total, ActiveImageCount: c.Active,
		})
		if t.Active {
			out.ActivePoolSize += c.Active
		}
	}
	return out, nil
}

// AdminCellImage is the admin view of an image; previewUrl is admin-only.
type AdminCellImage struct {
	ID         string    `json:"id"`
	TypeKey    string    `json:"typeKey"`
	PreviewURL string    `json:"previewUrl"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ListImagesInput drives GET /admin/cell-images.
type ListImagesInput struct {
	TypeKey  string
	Active   *bool
	Page     int
	PageSize int
}

func (s *CellLibraryService) ListImages(ctx context.Context, in ListImagesInput) (*Paged[AdminCellImage], error) {
	page, size := normalizePage(in.Page, in.PageSize)
	list, total, err := s.images.List(ctx, port.ImageFilter{TypeKey: in.TypeKey, Active: in.Active}, port.Page{Page: page, PageSize: size})
	if err != nil {
		return nil, internal(err)
	}
	items := make([]AdminCellImage, 0, len(list))
	for _, im := range list {
		items = append(items, toAdminImage(im, s.store.PreviewURL(im.Path)))
	}
	return &Paged[AdminCellImage]{Items: items, Page: page, PageSize: size, Total: total}, nil
}

func toAdminImage(im entity.CellImage, preview string) AdminCellImage {
	return AdminCellImage{ID: im.ID.Hex(), TypeKey: im.TypeKey, PreviewURL: preview, Active: im.Active, CreatedAt: im.CreatedAt}
}
