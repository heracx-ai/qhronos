package models

import (
	"time"

	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
)

type EventStatus string

const (
	EventStatusActive   EventStatus = "active"
	EventStatusInactive EventStatus = "inactive"
	EventStatusPaused   EventStatus = "paused"
	EventStatusDeleted  EventStatus = "deleted"
)

// ScheduleConfig represents the JSON schedule configuration
type ScheduleConfig struct {
	Frequency  string   `json:"frequency"`              // daily, weekly, monthly, yearly
	Interval   int      `json:"interval"`               // interval between occurrences
	ByDay      []string `json:"by_day,omitempty"`       // for weekly frequency
	ByMonth    []int    `json:"by_month,omitempty"`     // for yearly frequency
	ByMonthDay []int    `json:"by_month_day,omitempty"` // for monthly frequency
	Count      *int     `json:"count,omitempty"`        // number of occurrences
	Until      *string  `json:"until,omitempty"`        // end date in RFC3339 format
}

// Implement driver.Valuer for ScheduleConfig
func (s *ScheduleConfig) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Implement sql.Scanner for ScheduleConfig
func (s *ScheduleConfig) Scan(value interface{}) error {
	if value == nil {
		*s = ScheduleConfig{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("failed to scan ScheduleConfig: %v", value)
	}
}

// Event represents a scheduled event
type Event struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	StartTime   time.Time       `json:"start_time" db:"start_time"`
	WebhookURL  string          `json:"webhook_url" db:"webhook_url"`
	Metadata    datatypes.JSON  `json:"metadata" db:"metadata"`
	Schedule    *ScheduleConfig `json:"schedule,omitempty" db:"schedule"`
	Tags        pq.StringArray  `json:"tags" db:"tags"`
	Status      EventStatus     `json:"status" db:"status"`
	HMACSecret  *string         `json:"hmac_secret,omitempty" db:"hmac_secret"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   *time.Time      `json:"updated_at,omitempty" db:"updated_at"`
}

type CreateEventRequest struct {
	Name        string          `json:"name" validate:"required"`
	Description string          `json:"description"`
	StartTime   time.Time       `json:"start_time" validate:"required"`
	WebhookURL  string          `json:"webhook_url" validate:"required,url"`
	Metadata    datatypes.JSON  `json:"metadata" validate:"required"`
	Schedule    *ScheduleConfig `json:"schedule,omitempty"`
	Tags        []string        `json:"tags"`
	HMACSecret  *string         `json:"hmac_secret,omitempty"`
}

type UpdateEventRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	StartTime   *time.Time      `json:"start_time,omitempty"`
	WebhookURL  *string         `json:"webhook_url,omitempty"`
	Metadata    datatypes.JSON  `json:"metadata,omitempty"`
	Schedule    *ScheduleConfig `json:"schedule,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Status      *string         `json:"status,omitempty"`
	HMACSecret  *string         `json:"hmac_secret,omitempty"`
}

type EventFilter struct {
	Tags            []string
	Status          EventStatus
	StartTimeBefore *time.Time
	StartTimeAfter  *time.Time
}
