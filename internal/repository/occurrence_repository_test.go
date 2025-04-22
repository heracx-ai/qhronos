package repository

import (
	"context"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOccurrenceRepository(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := NewEventRepository(db)
	occurrenceRepo := NewOccurrenceRepository(db)
	ctx := context.Background()

	// Add cleanup function at the start of TestOccurrenceRepository
	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	t.Run("Create and Get Occurrence", func(t *testing.T) {
		cleanup()
		// Create parent event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Test Event",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Create occurrence
		occurrence := &models.Occurrence{
			ID:           testutils.RandomUUID(),
			EventID:      event.ID,
			ScheduledAt:  testutils.RandomTime(),
			Status:       models.OccurrenceStatusPending,
			LastAttempt:  nil,
			AttemptCount: 0,
			CreatedAt:    time.Now(),
		}

		// Create occurrence
		err = occurrenceRepo.Create(ctx, occurrence)
		require.NoError(t, err)

		// Get occurrence
		retrieved, err := occurrenceRepo.GetByID(ctx, occurrence.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, occurrence.ID, retrieved.ID)
		assert.Equal(t, occurrence.EventID, retrieved.EventID)
		assert.Equal(t, occurrence.Status, retrieved.Status)
		assert.Equal(t, occurrence.AttemptCount, retrieved.AttemptCount)
	})

	t.Run("Update Occurrence Status", func(t *testing.T) {
		cleanup()
		// Create parent event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Test Event",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Create occurrence
		occurrence := &models.Occurrence{
			ID:           testutils.RandomUUID(),
			EventID:      event.ID,
			ScheduledAt:  testutils.RandomTime(),
			Status:       models.OccurrenceStatusPending,
			LastAttempt:  nil,
			AttemptCount: 0,
			CreatedAt:    time.Now(),
		}
		err = occurrenceRepo.Create(ctx, occurrence)
		require.NoError(t, err)

		// Update status
		err = occurrenceRepo.UpdateStatus(ctx, occurrence.ID, models.OccurrenceStatusCompleted)
		require.NoError(t, err)

		// Verify update
		retrieved, err := occurrenceRepo.GetByID(ctx, occurrence.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, models.OccurrenceStatusCompleted, retrieved.Status)
		assert.NotNil(t, retrieved.LastAttempt)
		assert.Equal(t, 1, retrieved.AttemptCount)
	})

	t.Run("List Occurrences by Tags", func(t *testing.T) {
		cleanup()
		// Create test events with occurrences
		events := []*models.Event{
			{
				ID:         testutils.RandomUUID(),
				Title:      "Event 1",
				StartTime:  testutils.RandomTime(),
				WebhookURL: "https://example.com/webhook1",
				Payload:    []byte(`{"key": "value1"}`),
				Tags:       []string{"test", "tag1"},
				Status:     models.EventStatusActive,
				CreatedAt:  time.Now(),
				UpdatedAt:  nil,
			},
			{
				ID:         testutils.RandomUUID(),
				Title:      "Event 2",
				StartTime:  testutils.RandomTime(),
				WebhookURL: "https://example.com/webhook2",
				Payload:    []byte(`{"key": "value2"}`),
				Tags:       []string{"test", "tag2"},
				Status:     models.EventStatusActive,
				CreatedAt:  time.Now(),
				UpdatedAt:  nil,
			},
		}

		for _, event := range events {
			err := eventRepo.Create(ctx, event)
			require.NoError(t, err)

			// Create occurrence for each event
			occurrence := &models.Occurrence{
				ID:           testutils.RandomUUID(),
				EventID:      event.ID,
				ScheduledAt:  testutils.RandomTime(),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			}
			err = occurrenceRepo.Create(ctx, occurrence)
			require.NoError(t, err)
		}

		// List occurrences with tag "test"
		filter := models.OccurrenceFilter{
			Tags:  []string{"test"},
			Page:  1,
			Limit: 10,
		}
		occurrences, total, err := occurrenceRepo.ListByTags(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, occurrences, 2)

		// List occurrences with tag "tag1"
		filter.Tags = []string{"tag1"}
		occurrences, total, err = occurrenceRepo.ListByTags(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, occurrences, 1)
	})

	t.Run("Non-existent Occurrence", func(t *testing.T) {
		cleanup()
		retrieved, err := occurrenceRepo.GetByID(ctx, testutils.RandomUUID())
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
} 