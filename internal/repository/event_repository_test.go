package repository

import (
	"context"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRepository(t *testing.T) {
	db := testutils.TestDB(t)
	repo := NewEventRepository(db)

	// Add cleanup function
	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	t.Run("Create and Get Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, event.ID, retrieved.ID)
		assert.Equal(t, event.Name, retrieved.Name)
		assert.Equal(t, event.Description, retrieved.Description)
		assert.Equal(t, event.StartTime.Unix(), retrieved.StartTime.Unix())
		assert.Equal(t, event.WebhookURL, retrieved.WebhookURL)
		assert.Equal(t, event.Metadata, retrieved.Metadata)
		assert.Equal(t, event.Schedule, retrieved.Schedule)
		assert.Equal(t, event.Tags, retrieved.Tags)
		assert.Equal(t, event.Status, retrieved.Status)
	})

	t.Run("Update Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		// Update event
		event.Name = "Updated Event"
		event.Description = "Updated Description"
		event.WebhookURL = "https://example.com/updated"
		event.Metadata = []byte(`{"key": "updated"}`)
		event.Schedule = testutils.StringPtr("FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1")
		event.Tags = pq.StringArray{"updated"}
		event.Status = models.EventStatusInactive

		err = repo.Update(context.Background(), event)
		require.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, "Updated Event", retrieved.Name)
		assert.Equal(t, "Updated Description", retrieved.Description)
		assert.Equal(t, "https://example.com/updated", retrieved.WebhookURL)
		assert.Equal(t, []byte(`{"key": "updated"}`), retrieved.Metadata)
		assert.Equal(t, testutils.StringPtr("FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1"), retrieved.Schedule)
		assert.Equal(t, pq.StringArray{"updated"}, retrieved.Tags)
		assert.Equal(t, models.EventStatusInactive, retrieved.Status)
	})

	t.Run("Delete Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		err = repo.Delete(context.Background(), event.ID)
		require.NoError(t, err)

		// Verify deletion
		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("List Events", func(t *testing.T) {
		cleanup()
		events := []*models.Event{
			{
				ID:          uuid.New(),
				Name:        "Event 1",
				Description: "Description 1",
				StartTime:   time.Now(),
				WebhookURL:  "https://example.com/webhook1",
				Metadata:    []byte(`{"key": "value1"}`),
				Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
				Tags:        pq.StringArray{"test1"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Event 2",
				Description: "Description 2",
				StartTime:   time.Now().Add(time.Hour),
				WebhookURL:  "https://example.com/webhook2",
				Metadata:    []byte(`{"key": "value2"}`),
				Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1"),
				Tags:        pq.StringArray{"test2"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
		}

		for _, e := range events {
			err := repo.Create(context.Background(), e)
			require.NoError(t, err)
		}

		// List all events
		retrieved, err := repo.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, retrieved, 2)
	})

	t.Run("List Events by Tags", func(t *testing.T) {
		cleanup()

		events := []*models.Event{
			{
				ID:          uuid.New(),
				Name:        "Event 1",
				Description: "Description 1",
				StartTime:   time.Now(),
				WebhookURL:  "https://example.com/webhook1",
				Metadata:    []byte(`{"key": "value1"}`),
				Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
				Tags:        pq.StringArray{"test", "tag1"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Event 2",
				Description: "Description 2",
				StartTime:   time.Now(),
				WebhookURL:  "https://example.com/webhook2",
				Metadata:    []byte(`{"key": "value2"}`),
				Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=TU,TH;INTERVAL=1"),
				Tags:        pq.StringArray{"test", "tag2"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
		}

		for _, event := range events {
			err := repo.Create(context.Background(), event)
			require.NoError(t, err)
		}

		// List events with tag "test"
		retrieved, err := repo.ListByTags(context.Background(), []string{"test"})
		require.NoError(t, err)
		assert.Len(t, retrieved, 2)

		// List events with tag "tag1"
		retrieved, err = repo.ListByTags(context.Background(), []string{"tag1"})
		require.NoError(t, err)
		assert.Len(t, retrieved, 1)
		assert.Equal(t, "Event 1", retrieved[0].Name)
	})

	t.Run("Non-existent Event", func(t *testing.T) {
		cleanup()
		retrieved, err := repo.GetByID(context.Background(), uuid.New())
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("Delete Already Deleted Event", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Event to Delete",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		// First deletion
		err = repo.Delete(context.Background(), event.ID)
		require.NoError(t, err)

		// Second deletion attempt
		err = repo.Delete(context.Background(), event.ID)
		require.NoError(t, err)

		// Verify event is deleted
		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("Delete Event with Occurrences", func(t *testing.T) {
		cleanup()
		// Create event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Event with Occurrences",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create occurrence
		occurrence := &models.Occurrence{
			ID:           uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			LastAttempt:  nil,
			AttemptCount: 0,
			CreatedAt:    time.Now(),
		}

		occurrenceRepo := NewOccurrenceRepository(db)
		err = occurrenceRepo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		// Delete event
		err = repo.Delete(context.Background(), event.ID)
		require.NoError(t, err)

		// Verify event is deleted
		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)

		// Verify occurrence is also deleted (due to ON DELETE CASCADE)
		retrievedOccurrence, err := occurrenceRepo.GetByID(context.Background(), occurrence.ID)
		require.NoError(t, err)
		assert.Nil(t, retrievedOccurrence)
	})

	t.Run("Delete Event Updated At", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Event to Delete",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		// Delete event
		err = repo.Delete(context.Background(), event.ID)
		require.NoError(t, err)

		// Verify event is deleted
		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
} 