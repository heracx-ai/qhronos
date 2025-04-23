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
	"gorm.io/datatypes"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestEventHandler(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	handler := NewEventHandler(eventRepo)

	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
		require.NoError(t, err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.POST("/events", handler.CreateEvent)
	router.GET("/events/:id", handler.GetEvent)
	router.PUT("/events/:id", handler.UpdateEvent)
	router.DELETE("/events/:id", handler.DeleteEvent)
	router.GET("/events", handler.ListEventsByTags)

	t.Run("Create Event", func(t *testing.T) {
		cleanup()
		req := models.CreateEventRequest{
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "weekly",
				Interval:  1,
				ByDay:     []string{"MO", "WE", "FR"},
			},
			Tags: []string{"test"},
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
		assert.Equal(t, req.WebhookURL, response.WebhookURL)
		assertJSONEqual(t, req.Metadata, response.Metadata)
		assert.Equal(t, req.Schedule, response.Schedule)
		assert.ElementsMatch(t, req.Tags, response.Tags)
	})

	t.Run("Get Event", func(t *testing.T) {
		cleanup()
		// First, create an event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
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
		err := eventRepo.Create(context.Background(), event)
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
		assert.Equal(t, event.WebhookURL, response.WebhookURL)
		assertJSONEqual(t, event.Metadata, response.Metadata)
		assert.Equal(t, event.Schedule, response.Schedule)
		assert.ElementsMatch(t, event.Tags, response.Tags)
	})

	t.Run("Update Event", func(t *testing.T) {
		cleanup()
		// Create event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
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
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		updateReq := models.UpdateEventRequest{
			Name:        stringPtr("Updated Event"),
			Description: stringPtr("Updated Description"),
			WebhookURL:  stringPtr("https://example.com/updated"),
			Metadata:    datatypes.JSON([]byte(`{"key": "updated"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "daily",
				Interval:  1,
			},
			Tags:   []string{"updated"},
			Status: stringPtr(string(models.EventStatusInactive)),
		}
		body, err := json.Marshal(updateReq)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/events/"+event.ID.String(), bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, *updateReq.Name, response.Name)
		assert.Equal(t, *updateReq.Description, response.Description)
		assert.Equal(t, *updateReq.WebhookURL, response.WebhookURL)
		assertJSONEqual(t, updateReq.Metadata, response.Metadata)
		assert.Equal(t, updateReq.Schedule, response.Schedule)
		assert.ElementsMatch(t, updateReq.Tags, response.Tags)
		assert.Equal(t, models.EventStatusInactive, response.Status)
	})

	t.Run("Delete Event", func(t *testing.T) {
		cleanup()
		// Create event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
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
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify deletion
		retrieved, err := eventRepo.GetByID(context.Background(), event.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("List Events by Tags", func(t *testing.T) {
		cleanup()
		// Create events
		events := []*models.Event{
			{
				ID:          uuid.New(),
				Name:        "Event 1",
				Description: "Description 1",
				StartTime:   time.Now(),
				WebhookURL:  "https://example.com/webhook1",
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
				WebhookURL:  "https://example.com/webhook2",
				Metadata:    datatypes.JSON([]byte(`{"key": "value2"}`)),
				Schedule: &models.ScheduleConfig{
					Frequency: "daily",
					Interval:  1,
				},
				Tags:      pq.StringArray{"test", "tag2"},
				Status:    models.EventStatusActive,
				CreatedAt: time.Now(),
			},
		}
		for _, event := range events {
			err := eventRepo.Create(context.Background(), event)
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
		req := models.CreateEventRequest{
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Schedule: &models.ScheduleConfig{
				Frequency: "invalid",
				Interval:  1,
			},
			Tags: []string{"test"},
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
			Metadata:   datatypes.JSON([]byte(`{"key": "value"}`)),
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

	// Test: Create Event with malformed JSON
	t.Run("Create Event with Malformed JSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		// Missing closing brace
		malformedJSON := `{"name": "Bad Event", "webhook_url": "https://example.com", "start_time": "2024-03-20T00:00:00Z", "metadata": {"key": "value"}`
		r := httptest.NewRequest("POST", "/events", bytes.NewBufferString(malformedJSON))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "error")
	})

	// Test: Update Event with malformed JSON
	t.Run("Update Event with Malformed JSON", func(t *testing.T) {
		cleanup()
		// Create event to update
		event := &models.Event{
			ID:         uuid.New(),
			Name:       "Event to Update",
			StartTime:  time.Now(),
			WebhookURL: "https://example.com/webhook",
			Metadata:   datatypes.JSON([]byte(`{"key": "value"}`)),
			Tags:       pq.StringArray{"test"},
			Status:     models.EventStatusActive,
			CreatedAt:  time.Now(),
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		malformedJSON := `{"name": "Updated Name", "metadata": {"key": "value"}` // missing closing brace
		r := httptest.NewRequest("PUT", "/events/"+event.ID.String(), bytes.NewBufferString(malformedJSON))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "error")
	})
}

func stringPtr(s string) *string {
	return &s
}

func assertJSONEqual(t *testing.T, expected, actual datatypes.JSON) {
	var expectedMap, actualMap map[string]interface{}
	err1 := json.Unmarshal(expected, &expectedMap)
	err2 := json.Unmarshal(actual, &actualMap)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, expectedMap, actualMap)
}
