package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestOccurrenceHandler(t *testing.T) {
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)
	handler := NewOccurrenceHandler(eventRepo, occurrenceRepo)

	cleanup := func() {
		ctx := context.Background()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE events, occurrences CASCADE")
		require.NoError(t, err)
	}

	router := gin.Default()
	router.GET("/occurrences/:id", handler.GetOccurrence)
	router.GET("/events/:id/occurrences", handler.ListOccurrencesByEvent)
	router.GET("/occurrences", handler.ListOccurrencesByTags)

	t.Run("List Occurrences", func(t *testing.T) {
		cleanup()
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

		occurrences := []*models.Occurrence{
			{
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now(),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
				StatusCode:   0,
				ResponseBody: "",
				ErrorMessage: "",
				StartedAt:    time.Time{},
				CompletedAt:  time.Time{},
			},
			{
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now().Add(time.Hour),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
				StatusCode:   0,
				ResponseBody: "",
				ErrorMessage: "",
				StartedAt:    time.Time{},
				CompletedAt:  time.Time{},
			},
		}

		for _, occ := range occurrences {
			err := occurrenceRepo.Create(context.Background(), occ)
			require.NoError(t, err)
		}

		// List all occurrences
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences?tags=test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var paginatedResp models.PaginatedResponse
		err = json.Unmarshal(w.Body.Bytes(), &paginatedResp)
		require.NoError(t, err)
		data, ok := paginatedResp.Data.([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 2)
	})

	t.Run("List Occurrences by Event ID", func(t *testing.T) {
		cleanup()
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
				Tags:      pq.StringArray{"test1"},
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
					Frequency: "weekly",
					Interval:  1,
					ByDay:     []string{"TU", "TH"},
				},
				Tags:      pq.StringArray{"test2"},
				Status:    models.EventStatusActive,
				CreatedAt: time.Now(),
			},
		}

		for _, event := range events {
			err := eventRepo.Create(context.Background(), event)
			require.NoError(t, err)

			occurrences := []*models.Occurrence{
				{
					OccurrenceID: uuid.New(),
					EventID:      event.ID,
					ScheduledAt:  time.Now(),
					Status:       models.OccurrenceStatusPending,
					AttemptCount: 0,
					Timestamp:    time.Now(),
					StatusCode:   0,
					ResponseBody: "",
					ErrorMessage: "",
					StartedAt:    time.Time{},
					CompletedAt:  time.Time{},
				},
				{
					OccurrenceID: uuid.New(),
					EventID:      event.ID,
					ScheduledAt:  time.Now().Add(time.Hour),
					Status:       models.OccurrenceStatusPending,
					AttemptCount: 0,
					Timestamp:    time.Now(),
					StatusCode:   0,
					ResponseBody: "",
					ErrorMessage: "",
					StartedAt:    time.Time{},
					CompletedAt:  time.Time{},
				},
			}

			for _, occ := range occurrences {
				err := occurrenceRepo.Create(context.Background(), occ)
				require.NoError(t, err)
			}
		}

		// List occurrences for Event 1
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/events/"+events[0].ID.String()+"/occurrences", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var response []*models.Occurrence
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Len(t, response, 2)
		for _, occ := range response {
			assert.Equal(t, events[0].ID, occ.EventID)
		}
	})

	t.Run("Get Occurrence", func(t *testing.T) {
		cleanup()
		// Create parent event
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

		// Create occurrence
		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			AttemptCount: 0,
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: "",
			StartedAt:    time.Time{},
			CompletedAt:  time.Time{},
		}
		err = occurrenceRepo.Create(context.Background(), occurrence)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences/"+strconv.Itoa(occurrence.ID), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Occurrence
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, occurrence.OccurrenceID, response.OccurrenceID)
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
				StartTime:   time.Now(),
				WebhookURL:  "https://example.com/webhook2",
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
			err := eventRepo.Create(context.Background(), e)
			require.NoError(t, err)
		}

		// Create test occurrences
		occurrences := []*models.Occurrence{
			{
				OccurrenceID: uuid.New(),
				EventID:      events[0].ID,
				ScheduledAt:  time.Now(),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
				StatusCode:   0,
				ResponseBody: "",
				ErrorMessage: "",
				StartedAt:    time.Time{},
				CompletedAt:  time.Time{},
			},
			{
				OccurrenceID: uuid.New(),
				EventID:      events[0].ID,
				ScheduledAt:  time.Now().Add(time.Hour),
				Status:       models.OccurrenceStatusPending,
				AttemptCount: 0,
				Timestamp:    time.Now(),
				StatusCode:   0,
				ResponseBody: "",
				ErrorMessage: "",
				StartedAt:    time.Time{},
				CompletedAt:  time.Time{},
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
		nonExistentID := 999999
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/occurrences/"+strconv.Itoa(nonExistentID), nil)
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
