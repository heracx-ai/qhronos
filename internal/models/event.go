package models

import (
	"time"

	"github.com/google/uuid"
)

type EventStatus string

const (
	EventStatusActive  EventStatus = "active"
	EventStatusPaused  EventStatus = "paused"
	EventStatusDeleted EventStatus = "deleted"
)

type Event struct {
	ID          uuid.UUID   `json:"id" db:"id"`
	Title       string      `json:"title" db:"title"`
	StartTime   time.Time   `json:"start_time" db:"start_time"`
	WebhookURL  string      `json:"webhook_url" db:"webhook_url"`
	Payload     []byte      `json:"payload" db:"payload"`
	Recurrence  *string     `json:"recurrence,omitempty" db:"recurrence"`
	Tags        []string    `json:"tags" db:"tags"`
	Status      EventStatus `json:"status" db:"status"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   *time.Time  `json:"updated_at,omitempty" db:"updated_at"`
}

type CreateEventRequest struct {
	Title      string    `json:"title" validate:"required"`
	StartTime  time.Time `json:"start_time" validate:"required"`
	WebhookURL string    `json:"webhook_url" validate:"required,url"`
	Payload    []byte    `json:"payload" validate:"required"`
	Recurrence *string   `json:"recurrence,omitempty"`
	Tags       []string  `json:"tags"`
}

type UpdateEventRequest struct {
	Title      *string    `json:"title,omitempty"`
	StartTime  *time.Time `json:"start_time,omitempty"`
	WebhookURL *string    `json:"webhook_url,omitempty"`
	Payload    []byte     `json:"payload,omitempty"`
	Recurrence *string    `json:"recurrence,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Status     *string    `json:"status,omitempty"`
}

type EventFilter struct {
	Tags []string
} 