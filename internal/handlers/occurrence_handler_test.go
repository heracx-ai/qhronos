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
	handler := NewOccurrenceHandler(eventRepo, occurrenceRepo)

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
	router.GET("/events/:id/occurrences", handler.ListOccurrencesByEvent)
	router.GET("/occurrences", handler.ListOccurrencesByTags)

	t.Run("Get Occurrence", func(t *testing.T) {
		cleanup()
		// Create parent event
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

		// Create occurrence
		occurrence := &models.Occurrence{
			ID:          uuid.New(),
			EventID:     event.ID,
			ScheduledAt: time.Now(),
			Status:      models.OccurrenceStatusPending,
			CreatedAt:   time.Now(),
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

	t.Run("List Occurrences by Event", func(t *testing.T) {
		cleanup()
		// Create test events
		events := []*models.Event{
			{
				ID:          uuid.New(),
				Name:        "Event 1",
				Description: "Description 1",
				StartTime:   time.Now(),
				WebhookURL:  "https://example.com/webhook1",
				Metadata:    []byte(`{"key": "value1"}`),
				Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
				Tags:        []string{"test1"},
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
				Tags:        []string{"test2"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
		}

		for _, e := range events {
			err := eventRepo.Create(context.Background(), e)
			require.NoError(t, err)
		}

		// Create test occurrences
		occurrences := []*models.Occurrence{
			{
				ID:          uuid.New(),
				EventID:     events[0].ID,
				ScheduledAt: time.Now(),
				Status:      models.OccurrenceStatusPending,
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				EventID:     events[0].ID,
				ScheduledAt: time.Now().Add(time.Hour),
				Status:      models.OccurrenceStatusPending,
				CreatedAt:   time.Now(),
			},
		}

		for _, o := range occurrences {
			err := occurrenceRepo.Create(context.Background(), o)
			require.NoError(t, err)
		}

		// Test list occurrences
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/events/"+events[0].ID.String()+"/occurrences", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []*models.Occurrence
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response, 2)
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

func stringPtr(s string) *string {
	return &s
} 