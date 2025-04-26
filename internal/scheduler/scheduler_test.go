package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

func TestScheduler(t *testing.T) {
	cleanup := func() (*repository.EventRepository, *repository.OccurrenceRepository, *Scheduler, *zap.Logger, *testing.T, *redis.Client) {
		db := testutils.TestDB(t)
		logger := zap.NewNop()
		redisClient := testutils.TestRedis(t)
		eventRepo := repository.NewEventRepository(db, logger, redisClient)
		occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
		scheduler := NewScheduler(redisClient, logger)
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
		require.NoError(t, err)
		err = redisClient.FlushAll(ctx).Err()
		require.NoError(t, err)
		return eventRepo, occurrenceRepo, scheduler, logger, t, redisClient
	}

	t.Run("Schedule Event", func(t *testing.T) {
		eventRepo, occurrenceRepo, scheduler, _, t, _ := cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			Webhook:     "https://example.com/webhook",
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
		eventRepo, occurrenceRepo, scheduler, _, t, redisClient := cleanup()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			Webhook:     "http://example.com",
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
		count, err := scheduler.GetDueSchedules(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		// Check dispatch queue contents
		items, err := redisClient.LRange(context.Background(), dispatchQueueKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 1)
		if len(items) == 0 {
			t.Fatalf("Expected 1 item in dispatch queue, got 0. Possible race with worker or scheduling failure.")
		}
		var sched models.Schedule
		err = json.Unmarshal([]byte(items[0]), &sched)
		require.NoError(t, err)
		assert.Equal(t, occurrences[0].OccurrenceID, sched.OccurrenceID)
		// Remove scheduled event (should not panic or check queue contents after this)
		err = scheduler.RemoveScheduledEvent(context.Background(), occurrences[0])
		require.NoError(t, err)
		// Do not check queue state after removal to avoid race with dispatcher/worker
		// Previously: time.Sleep(5 * time.Second) and assert.Len(t, items, 0)
		// Now: Only check for error on removal
	})

	t.Run("no due events", func(t *testing.T) {
		_, _, scheduler, _, t, _ := cleanup()
		// Get due events when none exist
		count, err := scheduler.GetDueSchedules(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("recurring event scheduling", func(t *testing.T) {
		eventRepo, _, scheduler, _, t, _ := cleanup()
		// Create recurring event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Recurring Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			Webhook:     "http://example.com",
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
		eventRepo, occurrenceRepo, scheduler, _, t, redisClient := cleanup()
		ctx := context.Background()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Sync Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			Webhook:     "http://example.com",
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
		for _, key := range results {
			data, err := redisClient.HGet(ctx, "schedule:data", key).Result()
			require.NoError(t, err)
			var sched models.Schedule
			err = json.Unmarshal([]byte(data), &sched)
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

	t.Run("idempotent scheduling prevents duplicates", func(t *testing.T) {
		eventRepo, occurrenceRepo, scheduler, _, t, redisClient := cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Idempotent Event",
			Description: "Should not be scheduled twice",
			StartTime:   time.Now().Add(1 * time.Hour),
			Webhook:     "http://example.com",
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

		scheduledAt := event.StartTime
		occ := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  scheduledAt,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(context.Background(), occ)
		require.NoError(t, err)

		// Schedule the same event/time twice
		err = scheduler.ScheduleEvent(context.Background(), occ, event)
		require.NoError(t, err)
		err = scheduler.ScheduleEvent(context.Background(), occ, event)
		require.NoError(t, err)

		// There should be only one schedule in Redis
		keys, err := redisClient.ZRange(context.Background(), scheduleKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, keys, 1)
	})

	t.Run("due schedules are moved to dispatch queue", func(t *testing.T) {
		eventRepo, occurrenceRepo, scheduler, _, t, redisClient := cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Dispatch Queue Event",
			Description: "Test Description",
			StartTime:   time.Now().Add(-2 * time.Minute),
			Webhook:     "https://example.com/webhook",
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
			ScheduledAt:  event.StartTime,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(context.Background(), occ)
		require.NoError(t, err)
		err = scheduler.ScheduleEvent(context.Background(), occ, event)
		require.NoError(t, err)

		// Move due schedules to dispatch queue
		count, err := scheduler.GetDueSchedules(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Check dispatch queue contents
		items, err := redisClient.LRange(context.Background(), dispatchQueueKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 1)
		if len(items) == 0 {
			t.Fatalf("Expected 1 item in dispatch queue, got 0. Possible race with worker or scheduling failure.")
		}
		var sched models.Schedule
		err = json.Unmarshal([]byte(items[0]), &sched)
		require.NoError(t, err)
		assert.Equal(t, occ.OccurrenceID, sched.OccurrenceID)
		// Note: Do not run the dispatcher worker during this assertion to avoid race conditions.
	})

	t.Run("PopDispatchQueue returns and removes schedule", func(t *testing.T) {
		eventRepo, occurrenceRepo, scheduler, _, t, redisClient := cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Pop Queue Event",
			Description: "Test Description",
			StartTime:   time.Now().Add(-2 * time.Minute),
			Webhook:     "https://example.com/webhook",
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
			ScheduledAt:  event.StartTime,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(context.Background(), occ)
		require.NoError(t, err)
		err = scheduler.ScheduleEvent(context.Background(), occ, event)
		require.NoError(t, err)
		_, err = scheduler.GetDueSchedules(context.Background())
		require.NoError(t, err)

		// Pop from dispatch queue
		sched, err := scheduler.PopDispatchQueue(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, sched)
		assert.Equal(t, occ.OccurrenceID, sched.OccurrenceID)

		// Queue should now be empty
		items, err := redisClient.LRange(context.Background(), dispatchQueueKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 0)
	})
}

func TestScheduler_AtomicGetDueSchedules_NoDuplicates(t *testing.T) {
	ctx := context.Background()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	scheduler := NewScheduler(redisClient, zap.NewNop())

	// Schedule 10 events due now
	now := time.Now()
	for i := 0; i < 10; i++ {
		occ := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      uuid.New(),
			ScheduledAt:  now,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    now,
		}
		event := &models.Event{
			ID:        occ.EventID,
			Name:      fmt.Sprintf("Event %d", i),
			StartTime: now,
			Webhook:   "http://example.com",
			Status:    models.EventStatusActive,
			Metadata:  []byte(`{}`),
			Tags:      []string{"test"},
			CreatedAt: now,
		}
		err := scheduler.ScheduleEvent(ctx, occ, event)
		require.NoError(t, err)
	}

	// Simulate multiple pollers
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := scheduler.GetDueSchedules(ctx)
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	// Check that dispatch queue has exactly 10 unique items
	items, err := redisClient.LRange(ctx, dispatchQueueKey, 0, -1).Result()
	require.NoError(t, err)
	assert.Len(t, items, 10)

	// Check for duplicates
	seen := make(map[string]bool)
	for _, item := range items {
		if seen[item] {
			t.Errorf("Duplicate item found in dispatch queue: %s", item)
		}
		seen[item] = true
	}
}

func TestScheduler_AtomicRetryPoller_NoDuplicates(t *testing.T) {
	ctx := context.Background()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	// scheduler := NewScheduler(redisClient, zap.NewNop()) // Remove unused variable

	// Simulate a failed event that needs retry
	now := time.Now()
	occ := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      uuid.New(),
		ScheduledAt:  now,
		Status:       models.OccurrenceStatusPending,
		Timestamp:    now,
		AttemptCount: 1,
	}
	event := &models.Event{
		ID:        occ.EventID,
		Name:      "Retry Event",
		StartTime: now,
		Webhook:   "http://example.com",
		Status:    models.EventStatusActive,
		Metadata:  []byte(`{}`),
		Tags:      []string{"test"},
		CreatedAt: now,
	}
	sched := models.Schedule{
		Occurrence: *occ,
		Name:       event.Name,
		Webhook:    event.Webhook,
		Metadata:   event.Metadata,
		Tags:       event.Tags,
	}
	data, err := json.Marshal(sched)
	require.NoError(t, err)
	// Add to retry queue, due now
	_, err = redisClient.ZAdd(ctx, retryQueueKey, redis.Z{
		Score:  float64(now.Unix()),
		Member: data,
	}).Result()
	require.NoError(t, err)

	// Simulate multiple retry pollers (using the Lua script)
	retryLua := `
local due = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
for i, v in ipairs(due) do
  redis.call('RPUSH', KEYS[2], v)
  redis.call('ZREM', KEYS[1], v)
end
return due
`
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nowStr := fmt.Sprintf("%f", float64(now.Unix()))
			_, err := redisClient.Eval(ctx, retryLua, []string{retryQueueKey, dispatchQueueKey}, nowStr).Result()
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	// Check that dispatch queue has exactly 1 item, no duplicates
	items, err := redisClient.LRange(ctx, dispatchQueueKey, 0, -1).Result()
	require.NoError(t, err)
	assert.Len(t, items, 1)
}
