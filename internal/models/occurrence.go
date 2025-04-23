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
	ID           int              `json:"id" db:"id"`
	OccurrenceID uuid.UUID        `json:"occurrence_id" db:"occurrence_id"`
	EventID      uuid.UUID        `json:"event_id" db:"event_id"`
	ScheduledAt  time.Time        `json:"scheduled_at" db:"scheduled_at"`
	Status       OccurrenceStatus `json:"status" db:"status"`
	AttemptCount int              `json:"attempt_count" db:"attempt_count"`
	Timestamp    time.Time        `json:"timestamp" db:"timestamp"`
	StatusCode   int              `json:"status_code" db:"status_code"`
	ResponseBody string           `json:"response_body" db:"response_body"`
	ErrorMessage string           `json:"error_message" db:"error_message"`
	StartedAt    time.Time        `json:"started_at" db:"started_at"`
	CompletedAt  time.Time        `json:"completed_at" db:"completed_at"`
}

type OccurrenceFilter struct {
	Tags  []string
	Page  int
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
