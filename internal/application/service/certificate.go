package service

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
)

// CertificateService serves a user's own certificates (B-06). Issuing is done
// by AttemptService at submit time.
type CertificateService struct {
	certs port.CertificateRepository
}

func NewCertificateService(c port.CertificateRepository) *CertificateService {
	return &CertificateService{certs: c}
}

// CertificateView is §5.5 CertificateView.
type CertificateView struct {
	ID            string    `json:"id"`
	CertNo        string    `json:"certNo"`
	RecipientName string    `json:"recipientName"`
	HospitalName  string    `json:"hospitalName"`
	QuizTitle     string    `json:"quizTitle"`
	Percent       float64   `json:"percent"`
	IssuedAt      time.Time `json:"issuedAt"`
	Revoked       bool      `json:"revoked"`
}

func toCertView(c *entity.Certificate) CertificateView {
	return CertificateView{
		ID: c.ID.Hex(), CertNo: c.CertNo, RecipientName: c.RecipientName, HospitalName: c.HospitalName,
		QuizTitle: c.QuizTitle, Percent: c.Percent, IssuedAt: c.IssuedAt, Revoked: c.Revoked(),
	}
}

// ListMine returns only the caller's certificates.
func (s *CertificateService) ListMine(ctx context.Context, p *Principal) ([]CertificateView, error) {
	list, err := s.certs.ListByUser(ctx, p.UserID)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]CertificateView, 0, len(list))
	for i := range list {
		out = append(out, toCertView(&list[i]))
	}
	return out, nil
}

// GetMine returns one of the caller's certificates; someone else's is a 404 (BR-27).
func (s *CertificateService) GetMine(ctx context.Context, p *Principal, idHex string) (*CertificateView, error) {
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	c, err := s.certs.FindByID(ctx, id)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if c.UserID != p.UserID {
		return nil, notFound()
	}
	v := toCertView(c)
	return &v, nil
}
