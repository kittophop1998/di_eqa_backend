package service

import (
	"context"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
)

// HospitalService handles hospital query use cases.
type HospitalService struct {
	hospitals port.HospitalRepository
}

func NewHospitalService(h port.HospitalRepository) *HospitalService {
	return &HospitalService{hospitals: h}
}

func (s *HospitalService) List(ctx context.Context, query string) ([]entity.Hospital, error) {
	return s.hospitals.List(ctx, query)
}

func (s *HospitalService) GetByCode(ctx context.Context, code string) (*entity.Hospital, error) {
	h, err := s.hospitals.FindByCode(ctx, code)
	if err != nil {
		return nil, ErrNotFound
	}
	return h, nil
}
