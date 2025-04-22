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
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestEventHandler(t *testing.T) {
	db := testutils.TestDB(t)
	repo := repository.NewEventRepository(db)
	handler := NewEventHandler(repo)

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
		startTime := time.Now()
		req := models.CreateEventRequest{
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   startTime,
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        []string{"test"},
		}

		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, req.Name, response.Name)
		assert.Equal(t, req.Description, response.Description)
		assert.Equal(t, req.StartTime.Unix(), response.StartTime.Unix())
		assert.Equal(t, req.WebhookURL, response.WebhookURL)
		assert.Equal(t, req.Metadata, response.Metadata)
		assert.Equal(t, req.Schedule, response.Schedule)
		assert.Equal(t, pq.StringArray(req.Tags), response.Tags)
		assert.Equal(t, models.EventStatusActive, response.Status)
	})

	t.Run("Get Event", func(t *testing.T) {
		cleanup()
		startTime := time.Now()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   startTime,
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}
		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, event.ID, response.ID)
		assert.Equal(t, event.Name, response.Name)
		assert.Equal(t, event.Description, response.Description)
		assert.Equal(t, event.StartTime.Unix(), response.StartTime.Unix())
		assert.Equal(t, event.WebhookURL, response.WebhookURL)
		assert.Equal(t, event.Metadata, response.Metadata)
		assert.Equal(t, event.Schedule, response.Schedule)
		assert.Equal(t, event.Tags, response.Tags)
		assert.Equal(t, event.Status, response.Status)
	})

	t.Run("Update Event", func(t *testing.T) {
		cleanup()
		startTime := time.Now()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   startTime,
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}
		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		updateTime := startTime.Add(24 * time.Hour)
		req := models.UpdateEventRequest{
			Name:        testutils.StringPtr("Updated Event"),
			Description: testutils.StringPtr("Updated Description"),
			StartTime:   &updateTime,
			WebhookURL:  testutils.StringPtr("https://example.com/updated-webhook"),
			Metadata:    []byte(`{"key": "updated"}`),
			Schedule:    testutils.StringPtr("FREQ=DAILY;INTERVAL=1"),
			Tags:        []string{"updated"},
			Status:      testutils.StringPtr(string(models.EventStatusInactive)),
		}

		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/events/"+event.ID.String(), bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, *req.Name, response.Name)
		assert.Equal(t, *req.Description, response.Description)
		assert.Equal(t, req.StartTime.Unix(), response.StartTime.Unix())
		assert.Equal(t, *req.WebhookURL, response.WebhookURL)
		assert.Equal(t, req.Metadata, response.Metadata)
		assert.Equal(t, req.Schedule, response.Schedule)
		assert.Equal(t, pq.StringArray(req.Tags), response.Tags)
		assert.Equal(t, models.EventStatus(*req.Status), response.Status)
	})

	t.Run("Delete Event", func(t *testing.T) {
		cleanup()
		startTime := time.Now()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   startTime,
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
			Tags:        pq.StringArray{"test"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}
		err := repo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify event is deleted
		retrieved, err := repo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("List Events by Tags", func(t *testing.T) {
		cleanup()
		startTime := time.Now()
		// Create test events
		events := []*models.Event{
			{
				ID:          uuid.New(),
				Name:        "Test Event 1",
				Description: "Test Description 1",
				StartTime:   startTime,
				WebhookURL:  "https://example.com/webhook1",
				Metadata:    []byte(`{"key": "value1"}`),
				Schedule:    testutils.StringPtr("FREQ=WEEKLY;BYDAY=MO,WE,FR;INTERVAL=1"),
				Tags:        pq.StringArray{"test", "tag1"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Test Event 2",
				Description: "Test Description 2",
				StartTime:   startTime,
				WebhookURL:  "https://example.com/webhook2",
				Metadata:    []byte(`{"key": "value2"}`),
				Schedule:    testutils.StringPtr("FREQ=DAILY;INTERVAL=1"),
				Tags:        pq.StringArray{"test", "tag2"},
				Status:      models.EventStatusActive,
				CreatedAt:   time.Now(),
			},
		}

		for _, event := range events {
			err := repo.Create(context.Background(), event)
			require.NoError(t, err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/events?tags=test", nil)
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []models.Event
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response, 2)
	})

	t.Run("Create Event with Invalid Schedule", func(t *testing.T) {
		cleanup()
		startTime := time.Now()
		req := models.CreateEventRequest{
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   startTime,
			WebhookURL:  "https://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Schedule:    testutils.StringPtr("INVALID_SCHEDULE"),
			Tags:        []string{"test"},
		}

		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Create Event with Missing Required Fields", func(t *testing.T) {
		cleanup()
		req := models.CreateEventRequest{
			Name:       "Test Event",
			WebhookURL: "https://example.com/webhook",
			Metadata:   []byte(`{"key": "value"}`),
			Tags:       []string{"test"},
		}

		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
} 