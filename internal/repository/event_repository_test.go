package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

func setupTestDB(t *testing.T) *sqlx.DB {
	db := testutils.TestDB(t)

	// Clean up existing tables
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
	require.NoError(t, err)

	return db
}

func TestEventRepository(t *testing.T) {
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	namespace := testutils.GetRedisNamespace()
	repo := NewEventRepository(db, logger, redisClient, namespace)

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
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
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
		assert.Equal(t, event.Webhook, retrieved.Webhook)
		assertJSONEqual(t, event.Metadata, retrieved.Metadata)
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
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		// Update event
		event.Name = "Updated Event"
		event.Description = "Updated Description"
		event.Webhook = "https://example.com/updated"
		event.Metadata = datatypes.JSON([]byte(`{"key": "updated"}`))
		event.Schedule = &models.ScheduleConfig{
			Frequency: "weekly",
			Interval:  1,
			ByDay:     []string{"TU", "TH"},
		}
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
		assert.Equal(t, "https://example.com/updated", retrieved.Webhook)
		assertJSONEqual(t, datatypes.JSON([]byte(`{"key": "updated"}`)), retrieved.Metadata)
		assert.Equal(t, event.Schedule, retrieved.Schedule)
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
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
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
				Webhook:     "https://example.com/webhook1",
				Metadata:    datatypes.JSON([]byte(`{"key": "value1"}`)),
				Schedule: &models.ScheduleConfig{
					Frequency: "weekly",
					Interval:  1,
					ByDay:     []string{"MO", "WE", "FR"},
				},
				Tags:      pq.StringArray{"test1"},
				Status:    models.EventStatusActive,
				CreatedAt: time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Event 2",
				Description: "Description 2",
				StartTime:   time.Now().Add(time.Hour),
				Webhook:     "https://example.com/webhook2",
				Metadata:    datatypes.JSON([]byte(`{"key": "value2"}`)),
				Schedule: &models.ScheduleConfig{
					Frequency: "weekly",
					Interval:  1,
					ByDay:     []string{"TU", "TH"},
				},
				Tags:      pq.StringArray{"test2"},
				Status:    models.EventStatusActive,
				CreatedAt: time.Now(),
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
				Webhook:     "https://example.com/webhook1",
				Metadata:    datatypes.JSON([]byte(`{"key": "value1"}`)),
				Schedule: &models.ScheduleConfig{
					Frequency: "weekly",
					Interval:  1,
					ByDay:     []string{"MO", "WE", "FR"},
				},
				Tags:      pq.StringArray{"test", "tag1"},
				Status:    models.EventStatusActive,
				CreatedAt: time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Event 2",
				Description: "Description 2",
				StartTime:   time.Now(),
				Webhook:     "https://example.com/webhook2",
				Metadata:    datatypes.JSON([]byte(`{"key": "value2"}`)),
				Schedule: &models.ScheduleConfig{
					Frequency: "weekly",
					Interval:  1,
					ByDay:     []string{"TU", "TH"},
				},
				Tags:      pq.StringArray{"test", "tag2"},
				Status:    models.EventStatusActive,
				CreatedAt: time.Now(),
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
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
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
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
		}

		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create occurrence
		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			AttemptCount: 0,
			Timestamp:    time.Now(),
		}

		occurrenceRepo := NewOccurrenceRepository(db, logger)
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
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
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

func TestDeleteOldEvents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	namespace := testutils.GetRedisNamespace()
	repo := NewEventRepository(db, logger, redisClient, namespace)
	ctx := context.Background()

	// Create test data
	event1 := &models.Event{
		ID:          uuid.New(),
		Name:        "Old Event",
		Description: "This is an old event",
		StartTime:   time.Now().Add(-48 * time.Hour),
		Webhook:     "http://example.com",
		Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
		Tags:        pq.StringArray{"test"},
		Status:      models.EventStatusActive,
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		UpdatedAt:   timePtr(time.Now().Add(-48 * time.Hour)),
	}

	event2 := &models.Event{
		ID:          uuid.New(),
		Name:        "Recent Event",
		Description: "This is a recent event",
		StartTime:   time.Now().Add(-12 * time.Hour),
		Webhook:     "http://example.com",
		Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
		Tags:        pq.StringArray{"test"},
		Status:      models.EventStatusActive,
		CreatedAt:   time.Now().Add(-12 * time.Hour),
	}

	// Create events
	err := repo.Create(ctx, event1)
	assert.NoError(t, err)
	err = repo.Create(ctx, event2)
	assert.NoError(t, err)

	// Create occurrence for event2
	occurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      event2.ID,
		ScheduledAt:  time.Now().Add(24 * time.Hour),
		Status:       models.OccurrenceStatusPending,
		AttemptCount: 0,
		Timestamp:    time.Now(),
	}
	err = repo.CreateOccurrence(ctx, occurrence)
	assert.NoError(t, err)

	// Test deletion with cutoff 24 hours ago
	cutoff := time.Now().Add(-24 * time.Hour)
	err = repo.DeleteOldEvents(ctx, cutoff)
	assert.NoError(t, err)

	// Verify event1 was deleted
	event, err := repo.GetByID(ctx, event1.ID)
	assert.NoError(t, err)
	assert.Nil(t, event)

	// Verify event2 was not deleted (has future occurrence)
	event, err = repo.GetByID(ctx, event2.ID)
	assert.NoError(t, err)
	assert.NotNil(t, event)
}

func TestDeleteOldOccurrences(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	namespace := testutils.GetRedisNamespace()
	repo := NewEventRepository(db, logger, redisClient, namespace)
	ctx := context.Background()

	// Create test event
	event := &models.Event{
		ID:          uuid.New(),
		Name:        "Test Event",
		Description: "Test event for occurrences",
		StartTime:   time.Now(),
		Webhook:     "http://example.com",
		Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
		Tags:        pq.StringArray{"test"},
		Status:      models.EventStatusActive,
		CreatedAt:   time.Now(),
	}

	err := repo.Create(ctx, event)
	assert.NoError(t, err)

	// Create old occurrence
	oldOccurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      event.ID,
		ScheduledAt:  time.Now().Add(-48 * time.Hour),
		Status:       models.OccurrenceStatusCompleted,
		AttemptCount: 0,
		Timestamp:    time.Now().Add(-48 * time.Hour),
	}

	// Create recent occurrence
	recentOccurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      event.ID,
		ScheduledAt:  time.Now().Add(-12 * time.Hour),
		Status:       models.OccurrenceStatusCompleted,
		AttemptCount: 0,
		Timestamp:    time.Now().Add(-12 * time.Hour),
	}

	// Create occurrences
	err = repo.CreateOccurrence(ctx, oldOccurrence)
	assert.NoError(t, err)
	err = repo.CreateOccurrence(ctx, recentOccurrence)
	assert.NoError(t, err)

	// Test deletion with cutoff 24 hours ago
	cutoff := time.Now().Add(-24 * time.Hour)
	err = repo.DeleteOldOccurrences(ctx, cutoff)
	assert.NoError(t, err)

	// Verify old occurrence was deleted
	occurrence, err := repo.GetOccurrenceByID(ctx, oldOccurrence.ID)
	assert.NoError(t, err)
	assert.Nil(t, occurrence)

	// Verify recent occurrence was not deleted
	occurrence, err = repo.GetOccurrenceByID(ctx, recentOccurrence.ID)
	assert.NoError(t, err)
	assert.NotNil(t, occurrence)
}

// Helper to compare JSON content
func assertJSONEqual(t *testing.T, expected, actual datatypes.JSON) {
	var expectedMap, actualMap map[string]interface{}
	err1 := json.Unmarshal(expected, &expectedMap)
	err2 := json.Unmarshal(actual, &actualMap)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, expectedMap, actualMap)
}

func TestArchiveOldData(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	namespace := testutils.GetRedisNamespace()

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
	eventRepo := NewEventRepository(db, logger, redisClient, namespace)
	err := eventRepo.Create(ctx, oldEvent)
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
	occRepo := NewOccurrenceRepository(db, logger)
	err = occRepo.Create(ctx, oldOccurrence)
	require.NoError(t, err)

	// Call the archival function
	_, err = db.ExecContext(ctx, "SELECT archive_old_data($1)", "24 hours")
	require.NoError(t, err)

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
