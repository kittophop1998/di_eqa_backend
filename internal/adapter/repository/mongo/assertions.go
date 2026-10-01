package mongo

import "github.com/di-eqa/backend/internal/domain/port"

// Compile-time checks that every adapter satisfies its port.
var (
	_ port.UserRepository        = (*UserRepo)(nil)
	_ port.HospitalRepository    = (*HospitalRepo)(nil)
	_ port.CellTypeRepository    = (*CellTypeRepo)(nil)
	_ port.CellImageRepository   = (*CellImageRepo)(nil)
	_ port.QuizRepository        = (*QuizRepo)(nil)
	_ port.AssignmentRepository  = (*AssignmentRepo)(nil)
	_ port.AttemptRepository     = (*AttemptRepo)(nil)
	_ port.CertificateRepository = (*CertificateRepo)(nil)
	_ port.LegacyQuizRepository  = (*LegacyQuizRepo)(nil)
)
