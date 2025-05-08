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
	"github.com/feedloop/qhronos/internal/scheduler"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

func ptr[T any](v T) *T { return &v }

func TestEventHandler(t *testing.T) {
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	// Use test namespace for Redis keys in tests
	namespace := testutils.GetRedisNamespace()
	eventRepo := repository.NewEventRepository(db, logger, redisClient, namespace)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	schedulerService := scheduler.NewScheduler(redisClient, logger, namespace)
	expander := scheduler.NewExpander(
		schedulerService,
		eventRepo,
		occurrenceRepo,
		24*time.Hour,  // lookAheadDuration
		1*time.Minute, // expansionInterval
		0,             // gracePeriod
		logger,
	)
	handler := NewEventHandler(eventRepo, expander)

	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
		require.NoError(t, err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.Default()
	// Inject logger into context for all requests
	router.Use(func(c *gin.Context) {
		c.Set("logger", logger)
		c.Next()
	})
	router.POST("/events", handler.CreateEvent)
	router.GET("/events/:id", handler.GetEvent)
	router.PUT("/events/:id", handler.UpdateEvent)
	router.DELETE("/events/:id", handler.DeleteEvent)
	router.GET("/events", handler.ListEventsByTags)

	t.Run("Create Event", func(t *testing.T) {
		cleanup()
		req := models.CreateEventRequest{
			Name:        "Test Event",
			Description: ptr("Test Description"),
			StartTime:   time.Now(),
			Webhook:     "https://example.com/webhook",
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
		assert.Equal(t, *req.Description, response.Description)
		assert.Equal(t, req.Webhook, response.Webhook)
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
		assert.Equal(t, event.Webhook, response.Webhook)
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
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		updateReq := models.UpdateEventRequest{
			Name:        ptr("Updated Event"),
			Description: ptr("Updated Description"),
			Webhook:     ptr("https://example.com/updated"),
			Metadata:    datatypes.JSON([]byte(`{"key": "updated"}`)),
			Schedule:    &models.ScheduleConfig{Frequency: "daily", Interval: 1},
			Tags:        []string{"updated"},
			Status:      ptr(string(models.EventStatusInactive)),
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
		assert.Equal(t, "Updated Event", response.Name)
		assert.Equal(t, "Updated Description", response.Description)
		assert.Equal(t, "https://example.com/updated", response.Webhook)
		assertJSONEqual(t, datatypes.JSON([]byte(`{"key": "updated"}`)), response.Metadata)
		require.NotNil(t, response.Schedule)
		assert.Equal(t, models.ScheduleConfig{Frequency: "daily", Interval: 1}, *response.Schedule)
		assert.ElementsMatch(t, []string{"updated"}, response.Tags)
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
			Description: ptr("Test Description"),
			StartTime:   time.Now(),
			Webhook:     "https://example.com/webhook",
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
			Name:     "Test Event",
			Webhook:  "https://example.com/webhook",
			Metadata: datatypes.JSON([]byte(`{"key": "value"}`)),
			Tags:     []string{"test"},
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
		malformedJSON := `{"name": "Bad Event", "webhook": "https://example.com", "start_time": "2024-03-20T00:00:00Z", "metadata": {"key": "value"}`
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
			ID:        uuid.New(),
			Name:      "Event to Update",
			StartTime: time.Now(),
			Webhook:   "https://example.com/webhook",
			Metadata:  datatypes.JSON([]byte(`{"key": "value"}`)),
			Tags:      pq.StringArray{"test"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
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

	t.Run("Create Event Without Metadata", func(t *testing.T) {
		cleanup()
		req := map[string]interface{}{
			"name":        "No Metadata Event",
			"description": "Should default metadata to empty object",
			"start_time":  time.Now().Format(time.RFC3339),
			"webhook":     "https://example.com/webhook",
			"tags":        []string{"test"},
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
		assert.Equal(t, req["name"], response.Name)
		assertJSONEqual(t, datatypes.JSON([]byte(`{}`)), response.Metadata)
	})

	t.Run("Create Event With Minimal Required Fields", func(t *testing.T) {
		cleanup()
		req := map[string]interface{}{
			"name":       "Minimal Event",
			"start_time": time.Now().Format(time.RFC3339),
			"webhook":    "https://example.com/webhook",
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
		assert.Equal(t, req["name"], response.Name)
		assertJSONEqual(t, datatypes.JSON([]byte(`{}`)), response.Metadata)
	})

	t.Run("Create Event With Invalid Data", func(t *testing.T) {
		cleanup()
		// Missing name
		req := map[string]interface{}{
			"start_time": time.Now().Format(time.RFC3339),
			"webhook":    "https://example.com/webhook",
		}
		body, err := json.Marshal(req)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Invalid start_time
		req = map[string]interface{}{
			"name":       "Invalid StartTime",
			"start_time": "not-a-date",
			"webhook":    "https://example.com/webhook",
		}
		body, err = json.Marshal(req)
		require.NoError(t, err)
		w = httptest.NewRecorder()
		r = httptest.NewRequest("POST", "/events", bytes.NewBuffer(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Update Event With Partial Data", func(t *testing.T) {
		cleanup()
		// Create event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Partial Update Event",
			Description: "Original Description",
			StartTime:   time.Now(),
			Webhook:     "https://example.com/webhook",
			Metadata:    datatypes.JSON([]byte(`{"key": "value"}`)),
			Tags:        pq.StringArray{"original"},
			Status:      models.EventStatusActive,
			CreatedAt:   time.Now(),
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		updateReq := map[string]interface{}{
			"description": "Updated Description",
			"tags":        []string{"updated"},
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
		assert.Equal(t, "Updated Description", response.Description)
		assert.ElementsMatch(t, []string{"updated"}, response.Tags)
		assert.Equal(t, event.Name, response.Name)
		assert.Equal(t, event.Webhook, response.Webhook)
	})

	t.Run("List Events By Tag", func(t *testing.T) {
		cleanup()
		// Create events with different tags
		event1 := &models.Event{
			ID:        uuid.New(),
			Name:      "Tag Event 1",
			StartTime: time.Now(),
			Webhook:   "https://example.com/webhook",
			Metadata:  datatypes.JSON([]byte(`{"key": "value1"}`)),
			Tags:      pq.StringArray{"tag1"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
		}
		event2 := &models.Event{
			ID:        uuid.New(),
			Name:      "Tag Event 2",
			StartTime: time.Now(),
			Webhook:   "https://example.com/webhook",
			Metadata:  datatypes.JSON([]byte(`{"key": "value2"}`)),
			Tags:      pq.StringArray{"tag2"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
		}
		err := eventRepo.Create(context.Background(), event1)
		require.NoError(t, err)
		err = eventRepo.Create(context.Background(), event2)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/events?tags=tag1", nil)
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var response []models.Event
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response, 1)
		assert.Equal(t, "Tag Event 1", response[0].Name)
	})

	t.Run("Delete Event And Ensure Gone", func(t *testing.T) {
		cleanup()
		// Create event
		event := &models.Event{
			ID:        uuid.New(),
			Name:      "Delete Me",
			StartTime: time.Now(),
			Webhook:   "https://example.com/webhook",
			Metadata:  datatypes.JSON([]byte(`{"key": "value"}`)),
			Tags:      pq.StringArray{"delete"},
			Status:    models.EventStatusActive,
			CreatedAt: time.Now(),
		}
		err := eventRepo.Create(context.Background(), event)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Try to get the deleted event
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/events/"+event.ID.String(), nil)
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Create Event with action:webhook", func(t *testing.T) {
		cleanup()
		body := `{
			"name": "Action Webhook Event",
			"description": "Test event with webhook action",
			"start_time": "2025-01-01T00:00:00Z",
			"action": {
				"type": "webhook",
				"params": { "url": "https://example.com/webhook" }
			},
			"tags": ["api"]
		}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "Action Webhook Event", resp["name"])
		assert.Equal(t, "Test event with webhook action", resp["description"])
		action := resp["action"].(map[string]interface{})
		assert.Equal(t, "webhook", action["type"])
		params := action["params"].(map[string]interface{})
		assert.Equal(t, "https://example.com/webhook", params["url"])
	})

	t.Run("Create Event with action:websocket", func(t *testing.T) {
		cleanup()
		body := `{
			"name": "Action Websocket Event",
			"description": "Test event with websocket action",
			"start_time": "2025-01-01T00:00:00Z",
			"action": {
				"type": "websocket",
				"params": { "client_name": "client1" }
			},
			"tags": ["api"]
		}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "Action Websocket Event", resp["name"])
		assert.Equal(t, "Test event with websocket action", resp["description"])
		action := resp["action"].(map[string]interface{})
		assert.Equal(t, "websocket", action["type"])
		params := action["params"].(map[string]interface{})
		assert.Equal(t, "client1", params["client_name"])
	})

	t.Run("Update Event with new action", func(t *testing.T) {
		cleanup()
		// Create initial event
		body := `{
			"name": "Update Action Event",
			"description": "Initial event",
			"start_time": "2025-01-01T00:00:00Z",
			"action": {
				"type": "webhook",
				"params": { "url": "https://example.com/old" }
			},
			"tags": ["api"]
		}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/events", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		id := resp["id"].(string)
		// Update with new action
		updateBody := `{
			"action": {
				"type": "websocket",
				"params": { "client_name": "client2" }
			}
		}`
		w2 := httptest.NewRecorder()
		r2 := httptest.NewRequest("PUT", "/events/"+id, bytes.NewBufferString(updateBody))
		r2.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w2, r2)
		assert.Equal(t, http.StatusOK, w2.Code)
		var resp2 map[string]interface{}
		err = json.Unmarshal(w2.Body.Bytes(), &resp2)
		assert.NoError(t, err)
		action := resp2["action"].(map[string]interface{})
		assert.Equal(t, "websocket", action["type"])
		params := action["params"].(map[string]interface{})
		assert.Equal(t, "client2", params["client_name"])
	})
}

func assertJSONEqual(t *testing.T, expected, actual datatypes.JSON) {
	var expectedMap, actualMap map[string]interface{}
	err1 := json.Unmarshal(expected, &expectedMap)
	err2 := json.Unmarshal(actual, &actualMap)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, expectedMap, actualMap)
}
