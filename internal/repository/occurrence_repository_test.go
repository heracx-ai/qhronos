package repository

import (
	"context"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOccurrenceRepository(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := NewEventRepository(db)
	occurrenceRepo := NewOccurrenceRepository(db)

	// Add cleanup function
	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	t.Run("Create and Get Occurrence", func(t *testing.T) {
		cleanup()

		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        []string{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create test occurrence
		occurrence := &models.Occurrence{
			ID:          uuid.New(),
			EventID:     event.ID,
			ScheduledAt: time.Now(),
			Status:      models.OccurrenceStatusPending,
			CreatedAt:   time.Now(),
		}

		err = occurrenceRepo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		// Get occurrence
		retrieved, err := occurrenceRepo.GetByID(context.Background(), occurrence.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, occurrence.ID, retrieved.ID)
		assert.Equal(t, occurrence.EventID, retrieved.EventID)
		assert.Equal(t, occurrence.Status, retrieved.Status)
	})

	t.Run("List Occurrences by Event", func(t *testing.T) {
		cleanup()

		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        []string{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create test occurrences
		occurrences := []*models.Occurrence{
			{
				ID:          uuid.New(),
				EventID:     event.ID,
				ScheduledAt: time.Now(),
				Status:      models.OccurrenceStatusPending,
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				EventID:     event.ID,
				ScheduledAt: time.Now().Add(time.Hour),
				Status:      models.OccurrenceStatusPending,
				CreatedAt:   time.Now(),
			},
		}

		for _, o := range occurrences {
			err := occurrenceRepo.Create(context.Background(), o)
			require.NoError(t, err)
		}

		// List occurrences
		retrieved, err := occurrenceRepo.ListByEventID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Len(t, retrieved, 2)
	})

	t.Run("Update Occurrence Status", func(t *testing.T) {
		cleanup()

		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        []string{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create test occurrence
		occurrence := &models.Occurrence{
			ID:          uuid.New(),
			EventID:     event.ID,
			ScheduledAt: time.Now(),
			Status:      models.OccurrenceStatusPending,
			CreatedAt:   time.Now(),
		}

		err = occurrenceRepo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		// Update status
		occurrence.Status = models.OccurrenceStatusCompleted
		err = occurrenceRepo.Update(context.Background(), occurrence)
		require.NoError(t, err)

		// Verify update
		retrieved, err := occurrenceRepo.GetByID(context.Background(), occurrence.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, models.OccurrenceStatusCompleted, retrieved.Status)
	})
} 