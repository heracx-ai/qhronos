package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// Redis key for scheduled events
	ScheduleKey = "schedules"
	// Redis key for dispatch queue
	dispatchQueueKey = "dispatch:queue"
)

type Scheduler struct {
	redis       *redis.Client
	logger      *zap.Logger
	redisPrefix string
}

func NewScheduler(redis *redis.Client, logger *zap.Logger, prefix string) *Scheduler {
	return &Scheduler{
		redis:       redis,
		logger:      logger,
		redisPrefix: prefix,
	}
}

// ScheduleEvent schedules a single event occurrence using idempotent deterministic keys
func (s *Scheduler) ScheduleEvent(ctx context.Context, occurrence *models.Occurrence, event *models.Event) error {
	// Compose deterministic key
	key := fmt.Sprintf("schedule:%s:%d", event.ID.String(), occurrence.ScheduledAt.Unix())

	// Check if already scheduled
	exists, err := s.redis.HExists(ctx, s.redisPrefix+"schedule:data", key).Result()
	if err != nil {
		return fmt.Errorf("failed to check schedule existence: %w", err)
	}
	if exists {
		return nil // Already scheduled, skip
	}

	// Marshal Schedule to JSON
	sched := models.Schedule{
		Occurrence:  *occurrence,
		Name:        event.Name,
		Description: event.Description,
		Webhook:     event.Webhook,
		Metadata:    event.Metadata,
		Tags:        event.Tags,
	}
	data, err := json.Marshal(sched)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	// Add to Redis sorted set and hash
	score := float64(occurrence.ScheduledAt.Unix())
	_, err = s.redis.ZAdd(ctx, s.redisPrefix+ScheduleKey, redis.Z{
		Score:  score,
		Member: key,
	}).Result()
	if err != nil {
		return fmt.Errorf("failed to add schedule to sorted set: %w", err)
	}
	_, err = s.redis.HSet(ctx, s.redisPrefix+"schedule:data", key, data).Result()
	if err != nil {
		return fmt.Errorf("failed to add schedule to hash: %w", err)
	}
	return nil
}

// GetDueSchedules moves due schedules to the dispatch queue (idempotent version)
func (s *Scheduler) GetDueSchedules(ctx context.Context) (int, error) {
	// Lua script for atomic move from schedule set/hash to dispatch queue
	scheduleLua := `
local keys = redis.call('ZRANGEBYSCORE', KEYS[1], '0', ARGV[1])
local count = 0
for i, key in ipairs(keys) do
  local data = redis.call('HGET', KEYS[2], key)
  if data then
    redis.call('RPUSH', KEYS[3], data)
    redis.call('ZREM', KEYS[1], key)
    redis.call('HDEL', KEYS[2], key)
    count = count + 1
  end
end
return count
`

	now := fmt.Sprintf("%d", time.Now().Unix())
	res, err := s.redis.Eval(ctx, scheduleLua, []string{s.redisPrefix + ScheduleKey, s.redisPrefix + "schedule:data", s.redisPrefix + dispatchQueueKey}, now).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to atomically move due schedules: %w", err)
	}
	if n, ok := res.(int64); ok {
		return int(n), nil
	}
	return 0, nil
}

// PopDispatchQueue pops a schedule from the dispatch queue (for worker use)
func (s *Scheduler) PopDispatchQueue(ctx context.Context) (*models.Schedule, error) {
	data, err := s.redis.LPop(ctx, s.redisPrefix+dispatchQueueKey).Result()
	if err == redis.Nil {
		return nil, nil // No item
	} else if err != nil {
		return nil, fmt.Errorf("failed to pop from dispatch queue: %w", err)
	}
	var sched models.Schedule
	if err := json.Unmarshal([]byte(data), &sched); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schedule from dispatch queue: %w", err)
	}
	return &sched, nil
}

// RemoveScheduledEvent removes a scheduled event from Redis (idempotent version)
func (s *Scheduler) RemoveScheduledEvent(ctx context.Context, occurrence *models.Occurrence) error {
	key := fmt.Sprintf("schedule:%s:%d", occurrence.EventID.String(), occurrence.ScheduledAt.Unix())
	_, err := s.redis.ZRem(ctx, s.redisPrefix+ScheduleKey, key).Result()
	if err != nil {
		return fmt.Errorf("failed to remove schedule from sorted set: %w", err)
	}
	_, err = s.redis.HDel(ctx, s.redisPrefix+"schedule:data", key).Result()
	if err != nil {
		return fmt.Errorf("failed to remove schedule from hash: %w", err)
	}
	return nil
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
	err = s.redis.ZAdd(ctx, ScheduleKey, redis.Z{
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
	case "minutely":
		nextTime = event.StartTime.Add(time.Duration(schedule.Interval) * time.Minute)
	case "hourly":
		nextTime = event.StartTime.Add(time.Duration(schedule.Interval) * time.Hour)
	case "daily":
		nextTime = event.StartTime.AddDate(0, 0, schedule.Interval)
	case "weekly":
		nextTime = event.StartTime.AddDate(0, 0, 7*schedule.Interval)
	case "monthly":
		nextTime = event.StartTime.AddDate(0, schedule.Interval, 0)
	case "yearly":
		nextTime = event.StartTime.AddDate(schedule.Interval, 0, 0)
	default:
		return nil, fmt.Errorf("unsupported frequency: %s", schedule.Frequency)
	}
	if nextTime.Before(now) {
		nextTime = now
	}
	return &nextTime, nil
}
