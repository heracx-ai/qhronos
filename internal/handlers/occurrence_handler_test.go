package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOccurrenceHandler(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)
	handler := NewOccurrenceHandler(occurrenceRepo)

	// Add cleanup function
	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	router := gin.Default()
	router.GET("/occurrences/:id", handler.GetOccurrence)
	router.GET("/occurrences", handler.ListOccurrencesByTags)

	t.Run("Get Occurrence", func(t *testing.T) {
		cleanup()
		// Create parent event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Test Event",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		// Create occurrence
		occurrence := &models.Occurrence{
			ID:           testutils.RandomUUID(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			LastAttempt:  nil,
			AttemptCount: 0,
			CreatedAt:    time.Now(),
		}
		err = occurrenceRepo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences/"+occurrence.ID.String(), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Occurrence
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, occurrence.ID, response.ID)
		assert.Equal(t, occurrence.EventID, response.EventID)
		assert.Equal(t, occurrence.Status, response.Status)
	})

	t.Run("List Occurrences by Tags", func(t *testing.T) {
		cleanup()
		// Create test events with occurrences
		events := []*models.Event{
			{
				ID:         testutils.RandomUUID(),
				Title:      "Event 1",
				StartTime:  time.Now(),
				WebhookURL: "https://example.com/webhook1",
				Payload:    []byte(`{"key": "value1"}`),
				Tags:       []string{"test", "tag1"},
				Status:     models.EventStatusActive,
				CreatedAt:  time.Now(),
			},
			{
				ID:         testutils.RandomUUID(),
				Title:      "Event 2",
				StartTime:  time.Now(),
				WebhookURL: "https://example.com/webhook2",
				Payload:    []byte(`{"key": "value2"}`),
				Tags:       []string{"test", "tag2"},
				Status:     models.EventStatusActive,
				CreatedAt:  time.Now(),
			},
		}

		for _, event := range events {
			err := eventRepo.Create(context.Background(), event)
			require.NoError(t, err)

			// Create occurrence for each event
			occurrence := &models.Occurrence{
				ID:           testutils.RandomUUID(),
				EventID:      event.ID,
				ScheduledAt:  time.Now(),
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    time.Now(),
			}
			err = occurrenceRepo.Create(context.Background(), occurrence)
			require.NoError(t, err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences?tags=test", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, 2, response.Pagination.Total)
		assert.Len(t, response.Data, 2)
	})

	t.Run("Non-existent Occurrence", func(t *testing.T) {
		cleanup()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences/"+uuid.New().String(), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("List Occurrences with Invalid Tags", func(t *testing.T) {
		cleanup()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
} 