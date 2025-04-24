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
	"gorm.io/datatypes"
)

func TestScheduler(t *testing.T) {
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	scheduler := NewScheduler(redisClient, logger)

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
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  event.StartTime.Add(-time.Minute),
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(context.Background(), occ)
		require.NoError(t, err)
		err = scheduler.ScheduleEvent(context.Background(), occ, event)
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
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(-time.Minute),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
			},
			{
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(5 * time.Minute),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
			},
		}

		for _, occ := range occurrences {
			err = occurrenceRepo.Create(context.Background(), occ)
			require.NoError(t, err)

			// Schedule in Redis
			err = scheduler.ScheduleEvent(context.Background(), occ, event)
			require.NoError(t, err)
		}

		// Get due schedules
		dueSchedules, err := scheduler.GetDueSchedules(context.Background())
		require.NoError(t, err)
		assert.Len(t, dueSchedules, 1)
		assert.Equal(t, dueSchedules[0].OccurrenceID.String(), dueSchedules[0].OccurrenceID.String())
		assert.Equal(t, occurrences[0].ScheduledAt.Unix(), dueSchedules[0].ScheduledAt.Unix())

		// Remove scheduled event
		err = scheduler.RemoveScheduledEvent(context.Background(), occurrences[0])
		require.NoError(t, err)

		// Verify event was removed
		dueSchedules, err = scheduler.GetDueSchedules(context.Background())
		require.NoError(t, err)
		assert.Empty(t, dueSchedules)
	})

	t.Run("no due events", func(t *testing.T) {
		cleanup()
		// Get due events when none exist
		dueEvents, err := scheduler.GetDueSchedules(context.Background())
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
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(-2 * time.Minute),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
			},
			{
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(-1 * time.Minute),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
			},
		}

		for _, occ := range occurrences {
			err = occurrenceRepo.Create(ctx, occ)
			require.NoError(t, err)
			err = scheduler.ScheduleEvent(ctx, occ, event)
			require.NoError(t, err)
		}

		// Get all scheduled occurrences from Redis
		results, err := redisClient.ZRange(ctx, scheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, results, len(occurrences))

		// Unmarshal Redis results
		redisSchedMap := make(map[string]time.Time)
		for _, res := range results {
			var sched models.Schedule
			err := json.Unmarshal([]byte(res), &sched)
			require.NoError(t, err)
			redisSchedMap[sched.OccurrenceID.String()] = sched.ScheduledAt
		}

		// Get all occurrences from DB
		dbOccs, err := occurrenceRepo.ListByEventID(ctx, event.ID)
		require.NoError(t, err)
		assert.Len(t, dbOccs, len(occurrences))

		// Check DB → Redis
		for _, occ := range dbOccs {
			_, found := redisSchedMap[occ.OccurrenceID.String()]
			assert.True(t, found, "Occurrence %s in DB not found in Redis", occ.OccurrenceID)
		}

		// Check Redis → DB
		dbOccMap := make(map[string]time.Time)
		for _, occ := range dbOccs {
			dbOccMap[occ.OccurrenceID.String()] = occ.ScheduledAt
		}
		for id, sched := range redisSchedMap {
			dbSched, found := dbOccMap[id]
			assert.True(t, found, "Occurrence %s in Redis not found in DB", id)
			assert.Equal(t, dbSched.Unix(), sched.Unix(), "ScheduledAt mismatch for occurrence %s", id)
		}
	})
}
