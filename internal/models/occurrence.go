package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
)

// Qhronos Event → Schedule → Occurrence flow:
// - Event: User-defined intent/configuration, stored in the database.
// - Schedule: Actionable plan (in Redis) for when the event should be executed, created by the expander.
// - Occurrence: Created only after a scheduled event is executed (dispatched by the dispatcher). Represents the actual attempt to process (dispatch) the event at a specific time, with status and result information.

// Occurrence represents a single execution attempt of an event, created only after a scheduled event (from Redis) is executed by the dispatcher.
// For recurring events, multiple occurrences are generated as each scheduled execution is processed. Each occurrence tracks its status, attempts, and delivery history.

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

// Schedule is used for storing scheduled events in Redis with all event fields (no prefix)
type Schedule struct {
	Occurrence
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Webhook     string         `json:"webhook"`
	Metadata    datatypes.JSON `json:"metadata"`
	Tags        pq.StringArray `json:"tags"`
	// Add more event fields here if needed for dispatch
}
