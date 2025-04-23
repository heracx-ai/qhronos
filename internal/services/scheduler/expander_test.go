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
)

func TestEventExpander(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)
	redisClient := testutils.TestRedis(t)
	scheduler := NewScheduler(redisClient)

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

	expander := NewExpander(scheduler, eventRepo, occurrenceRepo, lookAheadDuration, expansionInterval)

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
			WebhookURL:  "http://example.com",
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
		results, err := redisClient.ZRange(ctx, scheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// Verify at least one occurrence was created
		var occurrence models.Occurrence
		err = json.Unmarshal([]byte(results[0]), &occurrence)
		require.NoError(t, err)
		assert.Equal(t, event.ID, occurrence.EventID)
	})

	t.Run("no recurring events", func(t *testing.T) {
		cleanup()
		// Run expansion
		err := expander.ExpandEvents(ctx)
		require.NoError(t, err)

		// Verify no occurrences were created
		results, err := redisClient.ZRange(ctx, scheduleKey, 0, -1).Result()
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
			WebhookURL:  "http://example.com",
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

		// Verify no occurrences were created in Redis
		results, err := redisClient.ZRange(ctx, scheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}
