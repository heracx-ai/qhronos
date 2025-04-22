package handlers

import (
	"bytes"
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

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestEventHandler(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	handler := NewEventHandler(eventRepo)

	// Add cleanup function
	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
	}

	router := gin.Default()
	router.POST("/events", handler.CreateEvent)
	router.GET("/events/:id", handler.GetEvent)
	router.PUT("/events/:id", handler.UpdateEvent)
	router.DELETE("/events/:id", handler.DeleteEvent)
	router.GET("/events", handler.ListEventsByTags)

	t.Run("Create Event", func(t *testing.T) {
		cleanup()
		reqBody := models.CreateEventRequest{
			Title:      "Test Event",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, reqBody.Title, response.Title)
		assert.Equal(t, reqBody.WebhookURL, response.WebhookURL)
		assert.Equal(t, reqBody.Tags, response.Tags)
		assert.Equal(t, models.EventStatusActive, response.Status)
	})

	t.Run("Get Event", func(t *testing.T) {
		cleanup()
		// Create test event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Test Event",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, event.ID, response.ID)
		assert.Equal(t, event.Title, response.Title)
	})

	t.Run("Update Event", func(t *testing.T) {
		cleanup()
		// Create test event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Original Title",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		updateReq := models.UpdateEventRequest{
			Title:  stringPtr("Updated Title"),
			Tags:   []string{"test", "updated"},
			Status: stringPtr(string(models.EventStatusPaused)),
		}

		body, err := json.Marshal(updateReq)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/events/"+event.ID.String(), bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", response.Title)
		assert.Equal(t, []string{"test", "updated"}, response.Tags)
		assert.Equal(t, models.EventStatusPaused, response.Status)
	})

	t.Run("Delete Event", func(t *testing.T) {
		cleanup()
		// Create test event
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Event to Delete",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  nil,
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("DELETE", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify event is deleted
		w = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("List Events by Tags", func(t *testing.T) {
		cleanup()
		// Create test events
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
				UpdatedAt:  nil,
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
				UpdatedAt:  nil,
			},
		}

		for _, event := range events {
			err := eventRepo.Create(context.Background(), event)
			require.NoError(t, err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/events?tags=test", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []models.Event
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response, 2)
	})

	t.Run("List Events by Tags - No Tags", func(t *testing.T) {
		cleanup()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/events", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Create Event with Invalid Payload", func(t *testing.T) {
		cleanup()
		reqBody := models.CreateEventRequest{
			Title:      "Test Event",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`invalid json`),
			Tags:       []string{"test"},
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Get Event with Invalid ID", func(t *testing.T) {
		cleanup()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/events/invalid-id", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Update Event with Invalid Payload", func(t *testing.T) {
		cleanup()
		// Create test event
		now := time.Now()
		event := &models.Event{
			ID:         testutils.RandomUUID(),
			Title:      "Original Title",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Payload:    []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
			UpdatedAt:  timePtr(now),
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		invalidPayload := []byte(`invalid json`)
		updateReq := models.UpdateEventRequest{
			Title:   stringPtr("Updated Title"),
			Payload: invalidPayload,
		}

		body, err := json.Marshal(updateReq)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/events/"+event.ID.String(), bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Update Non-existent Event", func(t *testing.T) {
		cleanup()
		updateReq := models.UpdateEventRequest{
			Title: stringPtr("Updated Title"),
		}

		body, err := json.Marshal(updateReq)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/events/"+uuid.New().String(), bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Delete Non-existent Event", func(t *testing.T) {
		cleanup()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("DELETE", "/events/"+uuid.New().String(), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func stringPtr(s string) *string {
	return &s
} 