package models

import (
	"time"

	"github.com/google/uuid"
)

type OccurrenceStatus string

const (
	OccurrenceStatusPending    OccurrenceStatus = "pending"
	OccurrenceStatusScheduled  OccurrenceStatus = "scheduled"
	OccurrenceStatusDispatched OccurrenceStatus = "dispatched"
	OccurrenceStatusCompleted  OccurrenceStatus = "completed"
	OccurrenceStatusFailed     OccurrenceStatus = "failed"
)

type Occurrence struct {
	ID           uuid.UUID        `json:"id" db:"id"`
	EventID      uuid.UUID        `json:"event_id" db:"event_id"`
	ScheduledAt  time.Time        `json:"scheduled_at" db:"scheduled_at"`
	Status       OccurrenceStatus `json:"status" db:"status"`
	LastAttempt  *time.Time       `json:"last_attempt,omitempty" db:"last_attempt"`
	AttemptCount int              `json:"attempt_count" db:"attempt_count"`
	CreatedAt    time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt    *time.Time       `json:"updated_at,omitempty" db:"updated_at"`
}

type OccurrenceFilter struct {
	Tags []string
	Page int
	Limit int
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination struct {
		Page  int `json:"page"`
		Limit int `json:"limit"`
		Total int `json:"total"`
	} `json:"pagination"`
} 