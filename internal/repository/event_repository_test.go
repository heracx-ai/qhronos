package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRepository(t *testing.T) {
	db := testutils.TestDB(t)
	repo := NewEventRepository(db)
	ctx := context.Background()

	// Clean up before each test
	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	t.Run("Create and Get Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Test Event",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test", "event"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}

		// Create event
		err := repo.Create(ctx, event)
		require.NoError(t, err)

		// Get event
		retrieved, err := repo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, event.ID, retrieved.ID)
		assert.Equal(t, event.Title, retrieved.Title)
		assert.Equal(t, event.WebhookURL, retrieved.WebhookURL)
		assert.Equal(t, event.Tags, retrieved.Tags)
		assert.Equal(t, event.Status, retrieved.Status)
	})

	t.Run("Update Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Original Title",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}

		err := repo.Create(ctx, event)
		require.NoError(t, err)

		// Update event
		event.Title = "Updated Title"
		event.Tags = []string{"test", "updated"}
		event.Status = models.EventStatusPaused
		now := time.Now()
		event.UpdatedAt = &now

		err = repo.Update(ctx, event)
		require.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "Updated Title", retrieved.Title)
		assert.Equal(t, []string{"test", "updated"}, retrieved.Tags)
		assert.Equal(t, models.EventStatusPaused, retrieved.Status)
		assert.NotNil(t, retrieved.UpdatedAt)
	})

	t.Run("Delete Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Event to Delete",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}

		err := repo.Create(ctx, event)
		require.NoError(t, err)

		// Delete event
		err = repo.Delete(ctx, event.ID)
		require.NoError(t, err)

		// Verify deletion
		retrieved, err := repo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("List Events by Tags", func(t *testing.T) {
		cleanup()
		// Create test events
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
			err := repo.Create(ctx, event)
			require.NoError(t, err)
		}

		// List events with tag "test"
		retrieved, err := repo.ListByTags(ctx, []string{"test"})
		require.NoError(t, err)
		assert.Len(t, retrieved, 2)

		// List events with tag "tag1"
		retrieved, err = repo.ListByTags(ctx, []string{"tag1"})
		require.NoError(t, err)
		assert.Len(t, retrieved, 1)
		assert.Equal(t, "Event 1", retrieved[0].Title)
	})

	t.Run("Non-existent Event", func(t *testing.T) {
		cleanup()
		retrieved, err := repo.GetByID(ctx, testutils.RandomUUID())
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("Delete Already Deleted Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Event to Delete",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}

		err := repo.Create(ctx, event)
		require.NoError(t, err)

		// First deletion
		err = repo.Delete(ctx, event.ID)
		require.NoError(t, err)

		// Second deletion attempt
		err = repo.Delete(ctx, event.ID)
		require.Error(t, err)
		assert.Equal(t, sql.ErrNoRows, err)
	})

	t.Run("Delete Event with Occurrences", func(t *testing.T) {
		cleanup()
		// Create event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Event with Occurrences",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}

		err := repo.Create(ctx, event)
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

		occurrenceRepo := NewOccurrenceRepository(db)
		err = occurrenceRepo.Create(ctx, occurrence)
		require.NoError(t, err)

		// Delete event
		err = repo.Delete(ctx, event.ID)
		require.NoError(t, err)

		// Verify event is deleted
		retrieved, err := repo.GetByID(ctx, event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)

		// Verify occurrence is also deleted (due to ON DELETE CASCADE)
		retrievedOccurrence, err := occurrenceRepo.GetByID(ctx, occurrence.ID)
		require.NoError(t, err)
		assert.Nil(t, retrievedOccurrence)
	})

	t.Run("Delete Event Updated At", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Event to Delete",
			StartTime:  testutils.RandomTime(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}

		err := repo.Create(ctx, event)
		require.NoError(t, err)

		// Delete event
		err = repo.Delete(ctx, event.ID)
		require.NoError(t, err)

		// Verify updated_at was set
		var updatedAt time.Time
		err = db.QueryRowContext(ctx, "SELECT updated_at FROM events WHERE id = $1", event.ID).Scan(&updatedAt)
		require.NoError(t, err)
		assert.False(t, updatedAt.IsZero())
	})
} 