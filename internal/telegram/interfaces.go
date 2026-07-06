package telegram

import (
	"context"

	"EpicScoreBot/internal/report"

	"github.com/google/uuid"
)

// ReportService defines the contract for generating PDF reports.
type ReportService interface {
	GenerateReport(ctx context.Context, data report.ReportData) (string, error)
}

// ScoringService defines the scoring business-logic contract.
type ScoringService interface {
	TryCompleteEpicScoring(ctx context.Context, epicID uuid.UUID) error
	TryCompleteRiskScoring(ctx context.Context, riskID uuid.UUID) error
}

// AIClient defines the AI question-answering contract.
type AIClient interface {
	Ask(ctx context.Context, question string) (string, error)
}
