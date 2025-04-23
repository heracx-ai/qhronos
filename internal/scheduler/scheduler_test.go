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
	"gorm.io/datatypes"
)

func TestScheduler(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)
	redisClient := testutils.TestRedis(t)
	scheduler := NewScheduler(redisClient)

	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
		require.NoError(t, err)
		err = redisClient.FlushAll(ctx).Err()
		require.NoError(t, err)
	}

	t.Run("Schedule Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "daily",
				Interval:  1,
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		occ := &models.Occurrence{
			ID:          uuid.New(),
			EventID:     event.ID,
			ScheduledAt: event.StartTime.Add(-time.Minute),
			Status:      models.OccurrenceStatusPending,
			CreatedAt:   time.Now(),
		}
		err = occurrenceRepo.Create(context.Background(), occ)
		require.NoError(t, err)
		err = scheduler.ScheduleEvent(context.Background(), occ)
		require.NoError(t, err)

		// Verify occurrences are created
		occurrences, err := occurrenceRepo.ListByEventID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.NotEmpty(t, occurrences)
	})

	t.Run("successful scheduling", func(t *testing.T) {
		cleanup()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com",
			Status:      models.EventStatusActive,
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "daily",
				Interval:  1,
			},
			Tags:      pq.StringArray{"test"},
			CreatedAt: time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create occurrences
		occurrences := []*models.Occurrence{
			{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(-time.Minute),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			},
			{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(5 * time.Minute),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			},
		}

		for _, occ := range occurrences {
			err = occurrenceRepo.Create(context.Background(), occ)
			require.NoError(t, err)

			// Schedule in Redis
			err = scheduler.ScheduleEvent(context.Background(), occ)
			require.NoError(t, err)
		}

		// Get due occurrences
		dueOccurrences, err := scheduler.GetDueOccurrence(context.Background())
		require.NoError(t, err)
		assert.Len(t, dueOccurrences, 1)
		assert.Equal(t, occurrences[0].ID, dueOccurrences[0].ID)
		assert.Equal(t, occurrences[0].ScheduledAt.Unix(), dueOccurrences[0].ScheduledAt.Unix())

		// Remove scheduled event
		err = scheduler.RemoveScheduledEvent(context.Background(), occurrences[0])
		require.NoError(t, err)

		// Verify event was removed
		dueOccurrences, err = scheduler.GetDueOccurrence(context.Background())
		require.NoError(t, err)
		assert.Empty(t, dueOccurrences)
	})

	t.Run("no due events", func(t *testing.T) {
		cleanup()
		// Get due events when none exist
		dueEvents, err := scheduler.GetDueOccurrence(context.Background())
		require.NoError(t, err)
		assert.Empty(t, dueEvents)
	})

	t.Run("recurring event scheduling", func(t *testing.T) {
		cleanup()
		// Create recurring event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Recurring Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com",
			Schedule: &models.ScheduleConfig{
				Frequency: "daily",
				Interval:  1,
			},
			Status:    models.EventStatusActive,
			Metadata:  datatypes.JSON([]byte(`{"key": "value"}`)),
			Tags:      pq.StringArray{"test"},
			CreatedAt: time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		// Schedule recurring event
		err = scheduler.ScheduleRecurringEvent(context.Background(), event)
		require.NoError(t, err)

		// Get recurring events
		events, err := scheduler.GetRecurringEvents(context.Background())
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, event.ID, events[0].ID)

		// Remove recurring event
		err = scheduler.RemoveRecurringEvent(context.Background(), event.ID)
		require.NoError(t, err)

		// Verify event was removed
		events, err = scheduler.GetRecurringEvents(context.Background())
		require.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("database and redis sync", func(t *testing.T) {
		cleanup()
		ctx := context.Background()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Sync Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com",
			Status:      models.EventStatusActive,
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "daily",
				Interval:  1,
			},
			Tags:      pq.StringArray{"test"},
			CreatedAt: time.Now(),
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Create and schedule occurrences
		occurrences := []*models.Occurrence{
			{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(-2 * time.Minute),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			},
			{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(-1 * time.Minute),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			},
		}

		for _, occ := range occurrences {
			err = occurrenceRepo.Create(ctx, occ)
			require.NoError(t, err)
			err = scheduler.ScheduleEvent(ctx, occ)
			require.NoError(t, err)
		}

		// Get all scheduled occurrences from Redis
		results, err := redisClient.ZRange(ctx, scheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, results, len(occurrences))

		// Unmarshal Redis results
		redisOccMap := make(map[string]time.Time)
		for _, res := range results {
			var occ models.Occurrence
			err := json.Unmarshal([]byte(res), &occ)
			require.NoError(t, err)
			redisOccMap[occ.ID.String()] = occ.ScheduledAt
		}

		// Get all occurrences from DB
		dbOccs, err := occurrenceRepo.ListByEventID(ctx, event.ID)
		require.NoError(t, err)
		assert.Len(t, dbOccs, len(occurrences))

		// Check DB → Redis
		for _, occ := range dbOccs {
			_, found := redisOccMap[occ.ID.String()]
			assert.True(t, found, "Occurrence %s in DB not found in Redis", occ.ID)
		}

		// Check Redis → DB
		dbOccMap := make(map[string]time.Time)
		for _, occ := range dbOccs {
			dbOccMap[occ.ID.String()] = occ.ScheduledAt
		}
		for id, sched := range redisOccMap {
			dbSched, found := dbOccMap[id]
			assert.True(t, found, "Occurrence %s in Redis not found in DB", id)
			assert.Equal(t, dbSched.Unix(), sched.Unix(), "ScheduledAt mismatch for occurrence %s", id)
		}
	})
}
