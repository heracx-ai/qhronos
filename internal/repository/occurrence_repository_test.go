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
	"github.com/lib/pq"
)

func TestOccurrenceRepository(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := NewEventRepository(db)
	repo := NewOccurrenceRepository(db)

	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
		require.NoError(t, err)
	}

	t.Run("Create and Get Occurrence", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		occurrence := &models.Occurrence{
			ID:           uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			LastAttempt:  nil,
			AttemptCount: 0,
			CreatedAt:    time.Now(),
		}

		err = repo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		retrieved, err := repo.GetByID(context.Background(), occurrence.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, occurrence.ID, retrieved.ID)
		assert.Equal(t, occurrence.EventID, retrieved.EventID)
		assert.Equal(t, occurrence.ScheduledAt.Unix(), retrieved.ScheduledAt.Unix())
		assert.Equal(t, occurrence.Status, retrieved.Status)
		assert.Equal(t, occurrence.LastAttempt, retrieved.LastAttempt)
		assert.Equal(t, occurrence.AttemptCount, retrieved.AttemptCount)
	})

	t.Run("Update Occurrence", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		occurrence := &models.Occurrence{
			ID:           uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			LastAttempt:  nil,
			AttemptCount: 0,
			CreatedAt:    time.Now(),
		}

		err = repo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		// Update occurrence
		lastAttempt := time.Now()
		occurrence.Status = models.OccurrenceStatusFailed
		occurrence.LastAttempt = &lastAttempt
		occurrence.AttemptCount = 1

		err = repo.Update(context.Background(), occurrence)
		require.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetByID(context.Background(), occurrence.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, models.OccurrenceStatusFailed, retrieved.Status)
		assert.Equal(t, lastAttempt.Unix(), retrieved.LastAttempt.Unix())
		assert.Equal(t, 1, retrieved.AttemptCount)
	})

	t.Run("List Occurrences by Event ID", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		occurrences := []*models.Occurrence{
			{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now(),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			},
			{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(time.Hour),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			},
		}

		for _, occ := range occurrences {
			err := repo.Create(context.Background(), occ)
			require.NoError(t, err)
		}

		// List occurrences for event
		retrieved, err := repo.ListByEventID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Len(t, retrieved, 2)
		for _, occ := range retrieved {
			assert.Equal(t, event.ID, occ.EventID)
		}
	})
} 