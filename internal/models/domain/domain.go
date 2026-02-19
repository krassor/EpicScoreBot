package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle status of an epic or risk.
type Status string

const (
	StatusNew     Status = "NEW"
	StatusScoring Status = "SCORING"
	StatusScored  Status = "SCORED"
)

// Team represents a development team.
type Team struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Role represents a team role (e.g. IT-leader, analyst, BE developer, etc.).
type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
}

// User represents a scoring participant.
type User struct {
	ID         uuid.UUID
	FirstName  string
	LastName   string
	TelegramID int64
	Weight     int // 0–100 percent
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Epic represents a development epic to be scored.
type Epic struct {
	ID          uuid.UUID
	Number      string
	Name        string
	Description string
	TeamID      uuid.UUID
	Status      Status
	FinalScore  *float64 // nullable until scored
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Risk represents a risk associated with an epic.
type Risk struct {
	ID            uuid.UUID
	Description   string
	EpicID        uuid.UUID
	Status        Status
	WeightedScore *float64 // nullable until scored
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EpicScore represents a single user's score for an epic under a specific role.
type EpicScore struct {
	ID        uuid.UUID
	EpicID    uuid.UUID
	UserID    uuid.UUID
	RoleID    uuid.UUID
	Score     int
	CreatedAt time.Time
}

// EpicRoleScore stores the weighted average score per role for an epic.
type EpicRoleScore struct {
	ID          uuid.UUID
	EpicID      uuid.UUID
	RoleID      uuid.UUID
	WeightedAvg float64
}

// RiskScore represents a single user's probability/impact assessment for a risk.
type RiskScore struct {
	ID          uuid.UUID
	RiskID      uuid.UUID
	UserID      uuid.UUID
	Probability int // 1–4
	Impact      int // 1–4
	CreatedAt   time.Time
}
