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
	Description string          `json:"description" db:"description,omitempty"`
	StartTime   time.Time       `json:"start_time" db:"start_time"`
	Webhook     string          `json:"webhook" db:"webhook"`
	Action      *Action         `json:"action" db:"action,omitempty"`
	Metadata    datatypes.JSON  `json:"metadata" db:"metadata,omitempty"`
	Schedule    *ScheduleConfig `json:"schedule" db:"schedule,omitempty"`
	Tags        pq.StringArray  `json:"tags" db:"tags,omitempty"`
	Status      EventStatus     `json:"status" db:"status"`
	HMACSecret  *string         `json:"hmac_secret" db:"hmac_secret,omitempty"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   *time.Time      `json:"updated_at" db:"updated_at,omitempty"`
}

type CreateEventRequest struct {
	Name        string          `json:"name" validate:"required"`
	Description *string         `json:"description,omitempty"`
	StartTime   time.Time       `json:"start_time" validate:"required"`
	Webhook     string          `json:"webhook" validate:"required"`
	Metadata    datatypes.JSON  `json:"metadata" validate:"required"`
	Schedule    *ScheduleConfig `json:"schedule,omitempty"`
	Tags        []string        `json:"tags"`
	HMACSecret  *string         `json:"hmac_secret,omitempty"`
}

type UpdateEventRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	StartTime   *time.Time      `json:"start_time,omitempty"`
	Webhook     *string         `json:"webhook,omitempty"`
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

// TableName specifies the database table name for GORM (if used)
// For sqlx, this is typically not needed in the struct itself.
/*func (Event) TableName() string {
	return "events"
}*/

// ToEventResponse converts an Event model to an EventResponse for API output.
// This can be expanded to include more fields or transform data as needed.
func (e *Event) ToEventResponse() EventResponse {
	return EventResponse{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		StartTime:   e.StartTime,
		Webhook:     e.Webhook,
		Action:      e.Action,
		Metadata:    e.Metadata,
		Schedule:    e.Schedule,
		Tags:        e.Tags,
		Status:      string(e.Status),
		HMACSecret:  e.HMACSecret,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// EventCreateRequest defines the structure for creating a new event
type EventCreateRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description *string         `json:"description,omitempty"`
	StartTime   time.Time       `json:"start_time" binding:"required"`
	Webhook     *string         `json:"webhook,omitempty"`  // Kept for backward compatibility
	Action      *Action         `json:"action,omitempty"`   // New action field
	Metadata    json.RawMessage `json:"metadata,omitempty"` // Use json.RawMessage for flexible metadata
	Schedule    *ScheduleConfig `json:"schedule,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	HMACSecret  *string         `json:"hmac_secret,omitempty"`
}

// EventUpdateRequest defines the structure for updating an existing event
// All fields are optional, so use pointers
type EventUpdateRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	StartTime   *time.Time      `json:"start_time,omitempty"`
	Webhook     *string         `json:"webhook,omitempty"`  // Kept for backward compatibility
	Action      *Action         `json:"action,omitempty"`   // New action field
	Metadata    json.RawMessage `json:"metadata,omitempty"` // Use json.RawMessage for flexible metadata
	Schedule    *ScheduleConfig `json:"schedule,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Status      *EventStatus    `json:"status,omitempty"`
	HMACSecret  *string         `json:"hmac_secret,omitempty"`
}

// EventResponse defines the structure for API responses for an event
type EventResponse struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	StartTime   time.Time       `json:"start_time"`
	Webhook     string          `json:"webhook"`
	Action      *Action         `json:"action,omitempty"`
	Metadata    datatypes.JSON  `json:"metadata,omitempty"`
	Schedule    *ScheduleConfig `json:"schedule,omitempty"`
	Tags        pq.StringArray  `json:"tags,omitempty"`
	Status      string          `json:"status"`
	HMACSecret  *string         `json:"hmac_secret,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   *time.Time      `json:"updated_at,omitempty"`
}
