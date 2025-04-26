package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEventExpander(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	scheduler := NewScheduler(redisClient, logger)

	// Add cleanup function
	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
		err = redisClient.FlushAll(ctx).Err()
		require.NoError(t, err)
	}

	// Test configuration
	lookAheadDuration := 24 * time.Hour
	expansionInterval := 5 * time.Minute
	gracePeriod := 2 * time.Minute

	expander := NewExpander(scheduler, eventRepo, occurrenceRepo, lookAheadDuration, expansionInterval, gracePeriod, logger)

	t.Run("successful event expansion", func(t *testing.T) {
		cleanup()
		// Create test event with start time in the future
		startTime := time.Now().Add(1 * time.Hour)

		// Create a daily schedule
		scheduleConfig := &models.ScheduleConfig{
			Frequency: "daily",
			Interval:  1,
		}

		var err error
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   startTime,
			Webhook:     "http://example.com",
			Schedule:    scheduleConfig,
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		err = eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Verify event was created with correct status
		dbEvent, err := eventRepo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		require.NotNil(t, dbEvent)
		require.Equal(t, models.EventStatusActive, dbEvent.Status)

		// Run expansion
		err = expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Get all events from Redis sorted set
		results, err := redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// Verify at least one occurrence was created
		var occurrence models.Occurrence
		data, err := redisClient.HGet(ctx, "schedule:data", results[0]).Result()
		require.NoError(t, err)
		err = json.Unmarshal([]byte(data), &occurrence)
		require.NoError(t, err)
		assert.Equal(t, event.ID, occurrence.EventID)
	})

	t.Run("no recurring events", func(t *testing.T) {
		cleanup()
		// Run expansion
		err := expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Verify no occurrences were created
		results, err := redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("event with invalid schedule", func(t *testing.T) {
		cleanup()
		// Create test event with invalid schedule (nil)
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now().Add(1 * time.Hour),
			Webhook:     "http://example.com",
			Schedule:    nil, // Simulate missing/invalid schedule
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Run expansion
		err = expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Verify one occurrence was created in Redis
		results, err := redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, results, 1)

		// Verify the occurrence is for the correct event and scheduled at the correct time
		var occurrence models.Occurrence
		data, err := redisClient.HGet(ctx, "schedule:data", results[0]).Result()
		require.NoError(t, err)
		err = json.Unmarshal([]byte(data), &occurrence)
		require.NoError(t, err)
		assert.Equal(t, event.ID, occurrence.EventID)
		assert.Equal(t, event.StartTime.Unix(), occurrence.ScheduledAt.Unix())
	})

	t.Run("non-recurring event schedules single occurrence", func(t *testing.T) {
		cleanup()
		// Create a non-recurring event (Schedule == nil) with a future StartTime
		startTime := time.Now().Add(2 * time.Hour)
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Non-Recurring Event",
			Description: "Should schedule one occurrence",
			StartTime:   startTime,
			Webhook:     "http://example.com",
			Schedule:    nil, // Non-recurring
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Run expansion
		err = expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Get all occurrences from Redis sorted set
		results, err := redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, results, 1)

		// Verify the occurrence is for the correct event and scheduled at the correct time
		var occurrence models.Occurrence
		data, err := redisClient.HGet(ctx, "schedule:data", results[0]).Result()
		require.NoError(t, err)
		err = json.Unmarshal([]byte(data), &occurrence)
		require.NoError(t, err)
		assert.Equal(t, event.ID, occurrence.EventID)
		assert.Equal(t, startTime.Unix(), occurrence.ScheduledAt.Unix())
	})

	t.Run("weekly_lookahead", func(t *testing.T) {
		cleanup()
		// Fixed time for deterministic test
		fixedNow := time.Date(2024, 5, 30, 10, 0, 0, 0, time.UTC) // Thursday
		lookAheadDuration := 14 * 24 * time.Hour                  // 2 weeks
		gracePeriod := 2 * time.Minute
		expansionInterval := 5 * time.Minute

		eventStart := fixedNow.AddDate(0, 0, -28) // 4 weeks ago (Thursday)
		scheduleConfig := &models.ScheduleConfig{
			Frequency: "weekly",
			Interval:  1,
			ByDay:     []string{"MO"}, // Every Monday
		}
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Weekly Event",
			Description: "Weekly event starting 4 weeks ago",
			StartTime:   eventStart,
			Webhook:     "http://example.com",
			Schedule:    scheduleConfig,
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   eventStart,
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Patch time.Now to return fixedNow
		oldNow := TimeNow
		TimeNow = func() time.Time { return fixedNow }
		defer func() { TimeNow = oldNow }()

		expander := NewExpander(scheduler, eventRepo, occurrenceRepo, lookAheadDuration, expansionInterval, gracePeriod, logger)
		err = expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Get all scheduled occurrences from Redis
		results, err := redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		var scheduledMondays []time.Time
		for _, key := range results {
			data, err := redisClient.HGet(ctx, "schedule:data", key).Result()
			require.NoError(t, err)
			var sched models.Schedule
			err = json.Unmarshal([]byte(data), &sched)
			require.NoError(t, err)
			if sched.EventID == event.ID && sched.ScheduledAt.Weekday() == time.Monday {
				scheduledMondays = append(scheduledMondays, sched.ScheduledAt)
			}
		}
		// Calculate expected Mondays in lookahead window
		expectedMondays := []time.Time{
			time.Date(2024, 6, 3, 10, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC),
		}
		assert.Equal(t, expectedMondays, scheduledMondays)
	})

	t.Run("update_event_cleans_and_reexpands", func(t *testing.T) {
		cleanup()
		fixedNow := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC) // Saturday
		lookAheadDuration := 14 * 24 * time.Hour                 // 2 weeks
		gracePeriod := 2 * time.Minute
		expansionInterval := 5 * time.Minute

		oldNow := TimeNow
		TimeNow = func() time.Time { return fixedNow }
		defer func() { TimeNow = oldNow }()

		eventStart := fixedNow.AddDate(0, 0, -7) // 1 week ago
		scheduleConfig := &models.ScheduleConfig{
			Frequency: "weekly",
			Interval:  1,
			ByDay:     []string{"MO"}, // Every Monday
		}
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Weekly Event",
			Description: "Weekly event for update test",
			StartTime:   eventStart,
			Webhook:     "http://example.com",
			Schedule:    scheduleConfig,
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   eventStart,
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		expander := NewExpander(scheduler, eventRepo, occurrenceRepo, lookAheadDuration, expansionInterval, gracePeriod, logger)
		err = expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Check that Monday is scheduled
		results, err := redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		var scheduledDays []time.Weekday
		for _, key := range results {
			data, err := redisClient.HGet(ctx, "schedule:data", key).Result()
			require.NoError(t, err)
			var sched models.Schedule
			err = json.Unmarshal([]byte(data), &sched)
			require.NoError(t, err)
			if sched.EventID == event.ID {
				scheduledDays = append(scheduledDays, sched.ScheduledAt.Weekday())
			}
		}
		assert.Contains(t, scheduledDays, time.Monday)
		assert.NotContains(t, scheduledDays, time.Wednesday)

		// Update event to schedule on Wednesday instead
		event.Schedule = &models.ScheduleConfig{
			Frequency: "weekly",
			Interval:  1,
			ByDay:     []string{"WE"}, // Every Wednesday
		}
		err = eventRepo.Update(ctx, event)
		require.NoError(t, err)
		// Remove old occurrences
		err = eventRepo.RemoveEventOccurrencesFromRedis(ctx, event.ID)
		require.NoError(t, err)
		// Re-expand
		err = expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Check that only Wednesday is scheduled
		results, err = redisClient.ZRange(ctx, ScheduleKey, 0, -1).Result()
		require.NoError(t, err)
		scheduledDays = scheduledDays[:0]
		for _, key := range results {
			data, err := redisClient.HGet(ctx, "schedule:data", key).Result()
			require.NoError(t, err)
			var sched models.Schedule
			err = json.Unmarshal([]byte(data), &sched)
			require.NoError(t, err)
			if sched.EventID == event.ID {
				scheduledDays = append(scheduledDays, sched.ScheduledAt.Weekday())
			}
		}
		assert.Contains(t, scheduledDays, time.Wednesday)
		assert.NotContains(t, scheduledDays, time.Monday)
	})
}
