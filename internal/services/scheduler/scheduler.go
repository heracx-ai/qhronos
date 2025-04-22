package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/redis/go-redis/v9"
	"github.com/google/uuid"
)

const (
	// Redis key for scheduled events
	scheduleKey = "schedule:events"
	// Redis key for recurring events
	recurringKey = "recurring:events"
)

type Scheduler struct {
	redis *redis.Client
}

func NewScheduler(redis *redis.Client) *Scheduler {
	return &Scheduler{redis: redis}
}

// ScheduleEvent schedules a single event occurrence
func (s *Scheduler) ScheduleEvent(ctx context.Context, occurrence *models.Occurrence) error {
	// Convert occurrence to JSON
	data, err := json.Marshal(occurrence)
	if err != nil {
		return fmt.Errorf("failed to marshal occurrence: %w", err)
	}

	// Add to Redis sorted set with scheduled_at as score
	_, err = s.redis.ZAdd(ctx, scheduleKey, redis.Z{
		Score:  float64(occurrence.ScheduledAt.Unix()),
		Member: data,
	}).Result()

	return err
}

// ScheduleRecurringEvent schedules a recurring event
func (s *Scheduler) ScheduleRecurringEvent(ctx context.Context, event *models.Event) error {
	if event.Schedule == nil {
		return fmt.Errorf("event has no schedule")
	}

	// Convert event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Add to Redis hash for recurring events
	_, err = s.redis.HSet(ctx, recurringKey, event.ID.String(), data).Result()
	return err
}

// GetDueEvents retrieves events that are due for execution
func (s *Scheduler) GetDueEvents(ctx context.Context, batchSize int64) ([]*models.Occurrence, error) {
	now := time.Now().Unix()

	// Get events due before now
	results, err := s.redis.ZRangeByScore(ctx, scheduleKey, &redis.ZRangeBy{
		Min:    "0",
		Max:    fmt.Sprintf("%d", now),
		Offset: 0,
		Count:  batchSize,
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get due events: %w", err)
	}

	var occurrences []*models.Occurrence
	for _, result := range results {
		var occurrence models.Occurrence
		if err := json.Unmarshal([]byte(result), &occurrence); err != nil {
			return nil, fmt.Errorf("failed to unmarshal occurrence: %w", err)
		}
		occurrences = append(occurrences, &occurrence)
	}

	return occurrences, nil
}

// RemoveScheduledEvent removes a scheduled event from Redis
func (s *Scheduler) RemoveScheduledEvent(ctx context.Context, occurrence *models.Occurrence) error {
	data, err := json.Marshal(occurrence)
	if err != nil {
		return fmt.Errorf("failed to marshal occurrence: %w", err)
	}

	_, err = s.redis.ZRem(ctx, scheduleKey, data).Result()
	return err
}

// GetRecurringEvents retrieves all recurring events
func (s *Scheduler) GetRecurringEvents(ctx context.Context) ([]*models.Event, error) {
	results, err := s.redis.HGetAll(ctx, recurringKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get recurring events: %w", err)
	}

	var events []*models.Event
	for _, result := range results {
		var event models.Event
		if err := json.Unmarshal([]byte(result), &event); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event: %w", err)
		}
		events = append(events, &event)
	}

	return events, nil
}

// RemoveRecurringEvent removes a recurring event from Redis
func (s *Scheduler) RemoveRecurringEvent(ctx context.Context, eventID uuid.UUID) error {
	_, err := s.redis.HDel(ctx, recurringKey, eventID.String()).Result()
	return err
} 