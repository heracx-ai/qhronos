package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/config"
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

func TestArchivalScheduler(t *testing.T) {
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	ctx := context.Background()

	// Clean up tables before test
	_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences, archived_events, archived_occurrences CASCADE")
	require.NoError(t, err)

	// Insert an old event and occurrence
	oldEvent := &models.Event{
		ID:          uuid.New(),
		Name:        "Old Event",
		Description: "Should be archived",
		StartTime:   time.Now().Add(-48 * time.Hour),
		Webhook:     "https://example.com/webhook",
		Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
		Schedule:    &models.ScheduleConfig{Frequency: "daily", Interval: 1},
		Tags:        pq.StringArray{"archive"},
		Status:      models.EventStatusActive,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
	}
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	err = eventRepo.Create(ctx, oldEvent)
	require.NoError(t, err)

	oldOccurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      oldEvent.ID,
		ScheduledAt:  time.Now().Add(-48 * time.Hour),
		Status:       models.OccurrenceStatusCompleted,
		AttemptCount: 1,
		Timestamp:    time.Now().Add(-48 * time.Hour),
		StatusCode:   200,
		ResponseBody: "ok",
		ErrorMessage: "",
		StartedAt:    time.Now().Add(-48 * time.Hour),
		CompletedAt:  time.Now().Add(-47 * time.Hour),
	}
	occRepo := repository.NewOccurrenceRepository(db, logger)
	err = occRepo.Create(ctx, oldOccurrence)
	require.NoError(t, err)

	// Start the archival scheduler with a short check period
	archivalStopCh := make(chan struct{})
	retention := config.RetentionConfig{
		Events:          "1d",
		Occurrences:     "1d",
		CleanupInterval: "1h",
	}
	durations, err := (&config.Config{Retention: retention}).ParseRetentionDurations()
	require.NoError(t, err)
	checkPeriod := 2 * time.Second
	StartArchivalScheduler(db, checkPeriod, *durations, archivalStopCh, logger)

	// Also call the archival function directly for immediate effect
	_, err = db.ExecContext(ctx, "SELECT archive_old_data($1)", "24 hours")
	require.NoError(t, err)

	// Wait for the archival to run
	time.Sleep(3 * time.Second)
	close(archivalStopCh)

	// Check that the old event and occurrence are gone from main tables
	e, err := eventRepo.GetByID(ctx, oldEvent.ID)
	assert.Nil(t, e)
	o, err := occRepo.GetLatestByOccurrenceID(ctx, oldOccurrence.OccurrenceID)
	assert.Nil(t, o)

	// Check that the event and occurrence are present in the archive tables
	var archivedEventCount int
	err = db.GetContext(ctx, &archivedEventCount, "SELECT COUNT(*) FROM archived_events WHERE event_id = $1", oldEvent.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, archivedEventCount)

	var archivedOccurrenceCount int
	err = db.GetContext(ctx, &archivedOccurrenceCount, "SELECT COUNT(*) FROM archived_occurrences WHERE occurrence_id = $1", oldOccurrence.OccurrenceID)
	assert.NoError(t, err)
	assert.Equal(t, 1, archivedOccurrenceCount)
}
