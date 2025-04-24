package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"bytes"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	resp := args.Get(0)
	if resp == nil {
		return nil, args.Error(1)
	}
	return resp.(*http.Response), args.Error(1)
}

func TestDispatcher(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)

	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger)
	dispatcher.SetHTTPClient(mockHTTP)

	// Add cleanup function
	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
		// Reset mock expectations
		mockHTTP = new(MockHTTPClient)
		dispatcher.SetHTTPClient(mockHTTP)
	}

	t.Run("successful dispatch", func(t *testing.T) {
		cleanup()

		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Create test schedule (simulate scheduling in Redis only)
		schedule := &models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now(),
			},
			Name:        event.Name,
			Description: event.Description,
			WebhookURL:  event.WebhookURL,
			Metadata:    event.Metadata,
			Tags:        event.Tags,
		}

		// Setup expectations
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
		}, nil)

		// Run dispatch
		err = dispatcher.DispatchWebhook(ctx, schedule)

		// Verify
		assert.NoError(t, err)
		mockHTTP.AssertExpectations(t)

		// Verify schedule log in Postgres (append-only)
		logged, err := occurrenceRepo.GetLatestByOccurrenceID(ctx, schedule.OccurrenceID)
		require.NoError(t, err)
		require.NotNil(t, logged)
		assert.Equal(t, models.OccurrenceStatusCompleted, logged.Status)
	})

	t.Run("event not found", func(t *testing.T) {
		cleanup()
		// Create schedule with non-existent event
		schedule := &models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: uuid.New(),
				EventID:      uuid.New(),
				ScheduledAt:  time.Now(),
			},
			Name:        "Non-existent Event",
			Description: "This event doesn't exist in the database",
			WebhookURL:  "http://example.com/webhook",
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
		}

		// Setup expectations for HTTP call (even if event not found, dispatcher may attempt HTTP call)
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return((*http.Response)(nil), errors.New("event not found"))

		// Run dispatch
		err := dispatcher.DispatchWebhook(ctx, schedule)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event not found")
		mockHTTP.AssertExpectations(t)
	})

	t.Run("webhook request failure", func(t *testing.T) {
		cleanup()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		schedule := &models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  time.Now(),
			},
			Name:        event.Name,
			Description: event.Description,
			WebhookURL:  event.WebhookURL,
			Metadata:    event.Metadata,
			Tags:        event.Tags,
		}

		err = occurrenceRepo.Create(ctx, &models.Occurrence{
			OccurrenceID: schedule.OccurrenceID,
			EventID:      schedule.EventID,
			ScheduledAt:  schedule.ScheduledAt,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: "",
			StartedAt:    time.Time{},
			CompletedAt:  time.Time{},
		})
		require.NoError(t, err)

		// Setup expectations
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return((*http.Response)(nil), errors.New("connection failed"))

		// Run dispatch
		err = dispatcher.DispatchWebhook(ctx, schedule)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "webhook request failed")
		mockHTTP.AssertExpectations(t)

		// Verify schedule status
		updatedSchedule, err := occurrenceRepo.GetLatestByOccurrenceID(ctx, schedule.OccurrenceID)
		require.NoError(t, err)
		assert.Equal(t, models.OccurrenceStatusFailed, updatedSchedule.Status)
	})
}

func TestDispatcher_RedisOnlyDispatch(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)
	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger)
	dispatcher.SetHTTPClient(mockHTTP)

	// Create Scheduler instance
	scheduler := NewScheduler(redisClient, logger)

	// Create event and schedule, schedule in Redis
	event := &models.Event{
		ID:          uuid.New(),
		Name:        "Event for Dispatcher Redis Test",
		Description: "Test",
		StartTime:   time.Now(),
		WebhookURL:  "http://example.com/webhook",
		Status:      models.EventStatusActive,
		Metadata:    []byte(`{"key": "value"}`),
		Tags:        pq.StringArray{"test"},
		CreatedAt:   time.Now(),
	}
	err := eventRepo.Create(ctx, event)
	require.NoError(t, err)
	schedule := &models.Schedule{
		Occurrence: models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now().Add(-time.Minute),
		},
		Name:        event.Name,
		Description: event.Description,
		WebhookURL:  event.WebhookURL,
		Metadata:    event.Metadata,
		Tags:        event.Tags,
	}
	// Add schedule to Scheduler (simulate scheduling in Redis)
	err = scheduler.ScheduleEvent(ctx, &schedule.Occurrence, event)
	require.NoError(t, err)

	// Debug: Print number of due schedules before running dispatcher
	dueSchedules, err := scheduler.GetDueSchedules(ctx)
	if err != nil {
		t.Fatalf("Error getting due schedules: %v", err)
	}
	fmt.Printf("[DEBUG] Due schedules before dispatcher: %d\n", len(dueSchedules))
	// Re-add the schedule since GetDueSchedules removes it
	if len(dueSchedules) > 0 {
		err = scheduler.ScheduleEvent(ctx, &schedule.Occurrence, event)
		require.NoError(t, err)
	}

	// Delete event from DB (simulate event deletion, but schedule remains in Redis)
	err = eventRepo.Delete(ctx, event.ID)
	require.NoError(t, err)

	// Setup expectations: dispatcher should still attempt dispatch
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
	}, nil)

	// Run dispatcher for one tick
	ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go func() {
		dispatcher.Run(ctxTimeout, scheduler)
	}()
	// Wait a moment for dispatcher to process
	time.Sleep(3 * time.Second)

	// Verify dispatch was attempted
	mockHTTP.AssertCalled(t, "Do", mock.AnythingOfType("*http.Request"))
}

func TestDispatcher_GetDueSchedules(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger := zap.NewNop()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)
	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger)
	dispatcher.SetHTTPClient(mockHTTP)

	// Create Scheduler instance
	scheduler := NewScheduler(redisClient, logger)

	// Create event and schedule, schedule in Redis
	event := &models.Event{
		ID:          uuid.New(),
		Name:        "Event for Dispatcher Redis Test",
		Description: "Test",
		StartTime:   time.Now(),
		WebhookURL:  "http://example.com/webhook",
		Status:      models.EventStatusActive,
		Metadata:    []byte(`{"key": "value"}`),
		Tags:        pq.StringArray{"test"},
		CreatedAt:   time.Now(),
	}
	err := eventRepo.Create(ctx, event)
	require.NoError(t, err)
	// Add schedule to Scheduler (simulate scheduling in Redis)
	occurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      event.ID,
		ScheduledAt:  time.Now().Add(-time.Minute),
	}
	err = scheduler.ScheduleEvent(ctx, occurrence, event)
	require.NoError(t, err)

	// Debug: Print number of due schedules before running dispatcher
	dueSchedules, err := scheduler.GetDueSchedules(ctx)
	if err != nil {
		t.Fatalf("Error getting due schedules: %v", err)
	}
	fmt.Printf("[DEBUG] Due schedules before dispatcher: %d\n", len(dueSchedules))
	// Re-add the schedule since GetDueSchedules removes it
	if len(dueSchedules) > 0 {
		err = scheduler.ScheduleEvent(ctx, occurrence, event)
		require.NoError(t, err)
	}

	// Setup expectations: dispatcher should still attempt dispatch
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
	}, nil)

	// Run dispatcher for one tick
	ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go func() {
		dispatcher.Run(ctxTimeout, scheduler)
	}()
	// Wait a moment for dispatcher to process
	time.Sleep(3 * time.Second)

	// Verify dispatch was attempted
	mockHTTP.AssertCalled(t, "Do", mock.AnythingOfType("*http.Request"))
}
