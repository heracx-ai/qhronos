package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
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
	// Use millisecond precision for the score to avoid collisions
	score := float64(occurrence.ScheduledAt.UnixMilli())
	_, err = s.redis.ZAdd(ctx, scheduleKey, redis.Z{
		Score:  score,
		Member: string(data),
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

// GetDueOccurrence retrieves occurrences that are due for execution
func (s *Scheduler) GetDueOccurrence(ctx context.Context) ([]*models.Occurrence, error) {
	now := time.Now().UnixMilli()

	// Get all occurrences due up to now
	results, err := s.redis.ZRangeByScore(ctx, scheduleKey, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get due occurrences from Redis: %w", err)
	}

	var occurrences []*models.Occurrence
	for _, result := range results {
		var occ models.Occurrence
		if err := json.Unmarshal([]byte(result), &occ); err != nil {
			return nil, fmt.Errorf("failed to unmarshal occurrence: %w", err)
		}
		occurrences = append(occurrences, &occ)

		// Remove the current occurrence
		if err := s.redis.ZRem(ctx, scheduleKey, result).Err(); err != nil {
			return nil, fmt.Errorf("failed to remove processed occurrence: %w", err)
		}
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

func (s *Scheduler) AddEvent(ctx context.Context, event *models.Event) error {
	if event.Schedule == nil {
		return fmt.Errorf("event has no schedule")
	}

	// Convert event to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Calculate next occurrence time
	nextTime, err := s.calculateNextOccurrence(event)
	if err != nil {
		return fmt.Errorf("failed to calculate next occurrence: %w", err)
	}

	// If no next occurrence, the event is complete
	if nextTime == nil {
		return nil
	}

	// Store event in Redis sorted set with score as Unix timestamp
	score := float64(nextTime.Unix())
	err = s.redis.ZAdd(ctx, scheduleKey, redis.Z{
		Score:  score,
		Member: string(eventJSON),
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to add event to Redis: %w", err)
	}

	return nil
}

// calculateNextOccurrence calculates the next occurrence time for an event based on its schedule
func (s *Scheduler) calculateNextOccurrence(event *models.Event) (*time.Time, error) {
	schedule := event.Schedule
	now := time.Now().UTC()

	// Handle different frequencies
	var nextTime time.Time
	switch schedule.Frequency {
	case "daily":
		nextTime = event.StartTime.AddDate(0, 0, schedule.Interval)
	case "weekly":
		nextTime = event.StartTime.AddDate(0, 0, 7*schedule.Interval)
	case "monthly":
		nextTime = event.StartTime.AddDate(0, schedule.Interval, 0)
	case "yearly":
		nextTime = event.StartTime.AddDate(schedule.Interval, 0, 0)
	default:
		return nil, fmt.Errorf("invalid frequency: %s", schedule.Frequency)
	}

	// Check if we've exceeded count
	if schedule.Count != nil {
		// TODO: Implement count check
		return nil, nil
	}

	// Check if we've exceeded until date
	if schedule.Until != nil {
		untilTime, err := time.Parse(time.RFC3339, *schedule.Until)
		if err != nil {
			return nil, fmt.Errorf("invalid until date: %w", err)
		}
		if nextTime.After(untilTime) {
			return nil, nil
		}
	}

	// Check if the next occurrence is in the past
	if nextTime.Before(now) {
		return nil, nil
	}

	return &nextTime, nil
}
