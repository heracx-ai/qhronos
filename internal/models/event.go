package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type EventStatus string

const (
	EventStatusActive   EventStatus = "active"
	EventStatusInactive EventStatus = "inactive"
	EventStatusPaused   EventStatus = "paused"
	EventStatusDeleted  EventStatus = "deleted"
)

type Event struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	Name        string         `json:"name" db:"name"`
	Description string         `json:"description" db:"description"`
	StartTime   time.Time      `json:"start_time" db:"start_time"`
	WebhookURL  string         `json:"webhook_url" db:"webhook_url"`
	Metadata    []byte         `json:"metadata" db:"metadata"`
	Schedule    *string        `json:"schedule,omitempty" db:"schedule"`
	Tags        pq.StringArray `json:"tags" db:"tags"`
	Status      EventStatus    `json:"status" db:"status"`
	HMACSecret  *string        `json:"hmac_secret,omitempty" db:"hmac_secret"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   *time.Time     `json:"updated_at,omitempty" db:"updated_at"`
}

type CreateEventRequest struct {
	Name        string    `json:"name" validate:"required"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time" validate:"required"`
	WebhookURL  string    `json:"webhook_url" validate:"required,url"`
	Metadata    []byte    `json:"metadata" validate:"required"`
	Schedule    *string   `json:"schedule,omitempty"`
	Tags        []string  `json:"tags"`
	HMACSecret  *string   `json:"hmac_secret,omitempty"`
}

type UpdateEventRequest struct {
	Name        *string    `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	WebhookURL  *string    `json:"webhook_url,omitempty"`
	Metadata    []byte     `json:"metadata,omitempty"`
	Schedule    *string    `json:"schedule,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Status      *string    `json:"status,omitempty"`
	HMACSecret  *string    `json:"hmac_secret,omitempty"`
}

type EventFilter struct {
	Tags []string
} 