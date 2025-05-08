package integration

import (
	"context"
	"encoding/json"
	"fmt"
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
)

func TestEventRepository_RedisCleanupOnDelete(t *testing.T) {
	ctx := context.Background()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	logger := zap.NewNop()
	// Use a real DB for event repo, but test Redis
	db := testutils.TestDB(t)
	namespace := testutils.GetRedisNamespace()
	repo := repository.NewEventRepository(db, logger, redisClient, namespace)

	// Create event and schedule occurrences in Redis (inline logic)
	event := &models.Event{
		ID:          uuid.New(),
		Name:        "Event for Redis Cleanup",
		Description: "Test",
		StartTime:   time.Now(),
		Webhook:     "http://example.com",
		Status:      models.EventStatusActive,
		Metadata:    []byte(`{"key": "value"}`),
		Tags:        pq.StringArray{"test"},
		CreatedAt:   time.Now(),
	}
	err := repo.Create(ctx, event)
	require.NoError(t, err)

	occurrences := []*models.Occurrence{
		{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now().Add(1 * time.Hour),
			Status:       models.OccurrenceStatusPending,
			AttemptCount: 0,
			Timestamp:    time.Now(),
		},
		{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now().Add(2 * time.Hour),
			Status:       models.OccurrenceStatusPending,
			AttemptCount: 0,
			Timestamp:    time.Now(),
		},
	}
	for _, occ := range occurrences {
		data, err := json.Marshal(occ)
		require.NoError(t, err)
		key := "schedule:" + event.ID.String() + ":" + fmt.Sprintf("%d", occ.ScheduledAt.Unix())
		score := float64(occ.ScheduledAt.UnixMilli())
		_, err = redisClient.ZAdd(ctx, namespace+"schedules", redis.Z{
			Score:  score,
			Member: key,
		}).Result()
		require.NoError(t, err)
		_, err = redisClient.HSet(ctx, namespace+"schedule:data", key, data).Result()
		require.NoError(t, err)
	}

	// Ensure occurrences are in Redis
	results, err := redisClient.ZRange(ctx, namespace+"schedules", 0, -1).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Delete event
	err = repo.Delete(ctx, event.ID)
	require.NoError(t, err)

	// Ensure occurrences for this event are removed from Redis
	results, err = redisClient.ZRange(ctx, namespace+"schedules", 0, -1).Result()
	require.NoError(t, err)
	for _, res := range results {
		// Fetch from hash instead of decoding directly
		data, err := redisClient.HGet(ctx, namespace+"schedule:data", res).Result()
		if err != nil {
			continue // skip if not found
		}
		var occ models.Occurrence
		err = json.Unmarshal([]byte(data), &occ)
		if err == nil {
			assert.NotEqual(t, event.ID, occ.EventID, "Occurrence for deleted event should be removed from Redis")
		}
	}
}
