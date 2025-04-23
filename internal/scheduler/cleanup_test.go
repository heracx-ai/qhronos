package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/lib/pq"
)

func TestCleanupService(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	ctx := context.Background()

	// Add cleanup function
	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	// Test configuration
	eventsRetention := 24 * time.Hour
	occurrencesRetention := 7 * 24 * time.Hour
	cleanupInterval := time.Hour

	service := NewCleanupService(eventRepo, eventsRetention, occurrencesRetention, cleanupInterval)

	t.Run("successful cleanup", func(t *testing.T) {
		cleanup()

		// Create test data
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Old Event",
			Description: "This is an old event",
			StartTime:   time.Now().Add(-48 * time.Hour),
			WebhookURL:  "http://example.com",
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now().Add(-48 * time.Hour),
			UpdatedAt:   timePtr(time.Now().Add(-48 * time.Hour)),
			Tags:        pq.StringArray{"test"},
			Metadata:    []byte(`{"key": "value"}`),
		}

		// Create event
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Create old occurrence
		occurrence := &models.Occurrence{
			ID:          uuid.New(),
			EventID:     event.ID,
			ScheduledAt: time.Now().Add(-48 * time.Hour),
			Status:      models.OccurrenceStatusCompleted,
			CreatedAt:   time.Now().Add(-48 * time.Hour),
		}

		// Create occurrence
		err = eventRepo.CreateOccurrence(ctx, occurrence)
		require.NoError(t, err)

		// Run cleanup
		err = service.cleanup(ctx)
		assert.NoError(t, err)

		// Verify event was deleted
		retrieved, err := eventRepo.GetByID(ctx, event.ID)
		assert.NoError(t, err)
		assert.Nil(t, retrieved)

		// Verify occurrence was deleted
		retrievedOccurrence, err := eventRepo.GetOccurrenceByID(ctx, occurrence.ID)
		assert.NoError(t, err)
		assert.Nil(t, retrievedOccurrence)
	})

	t.Run("service run with context cancellation", func(t *testing.T) {
		cleanup()
		ctx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		// Run service
		service.Run(ctx)
		// Should exit without error
	})

	t.Run("service run with cleanup interval", func(t *testing.T) {
		cleanup()
		ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		// Create service with short interval
		service := NewCleanupService(eventRepo, eventsRetention, occurrencesRetention, 50*time.Millisecond)
		
		// Run service
		service.Run(ctx)
	})

	t.Run("service run with zero retention periods", func(t *testing.T) {
		cleanup()
		// Create service with zero retention periods
		service := NewCleanupService(eventRepo, 0, 0, cleanupInterval)

		// Run cleanup
		err := service.cleanup(ctx)
		assert.NoError(t, err)
	})

	t.Run("service run with negative retention periods", func(t *testing.T) {
		cleanup()
		// Create service with negative retention periods
		service := NewCleanupService(eventRepo, -1*time.Hour, -1*time.Hour, cleanupInterval)

		// Run cleanup
		err := service.cleanup(ctx)
		assert.NoError(t, err)
	})
} 