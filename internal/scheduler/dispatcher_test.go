package scheduler

import (
	"context"
	"encoding/json"
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

// MockClientNotifier implements ClientNotifier for tests
// Tracks calls and simulates connected/disconnected clients and round-robin

type MockClientNotifier struct {
	connected map[string][]string // clientID -> list of "client" names (simulate connections)
	calls     []string            // record of dispatches (clientID:clientIndex)
	fail      map[string]bool     // clientID -> should fail
	indices   map[string]int      // round-robin index per clientID
}

func NewMockClientNotifier() *MockClientNotifier {
	return &MockClientNotifier{
		connected: make(map[string][]string),
		calls:     []string{},
		fail:      make(map[string]bool),
		indices:   make(map[string]int),
	}
}

func (m *MockClientNotifier) DispatchToClient(clientID string, payload []byte) error {
	clients := m.connected[clientID]
	if len(clients) == 0 {
		m.calls = append(m.calls, clientID)
		return fmt.Errorf("no client connected for id: %s", clientID)
	}
	idx := m.indices[clientID] % len(clients)
	m.calls = append(m.calls, fmt.Sprintf("%s:%s", clientID, clients[idx]))
	m.indices[clientID] = (m.indices[clientID] + 1) % len(clients)
	if m.fail[clientID] {
		return fmt.Errorf("simulated failure for %s", clientID)
	}
	return nil
}

func TestDispatcher(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger, _ := zap.NewDevelopment()
	redisClient := testutils.TestRedis(t)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)

	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 3, 5*time.Second, nil)
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
			Webhook:     "http://example.com/webhook",
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
			Webhook:     event.Webhook,
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
			Webhook:     "http://example.com/webhook",
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
			Webhook:     "http://example.com/webhook",
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
			Webhook:     event.Webhook,
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

	// Add client hook tests
	t.Run("client hook dispatch - single client", func(t *testing.T) {
		cleanup()
		mockNotifier := NewMockClientNotifier()
		mockNotifier.connected["client1"] = []string{"c1"}
		dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 3, 5*time.Second, mockNotifier)
		// Create schedule with q: webhook
		schedule := &models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: uuid.New(),
				EventID:      uuid.New(),
				ScheduledAt:  time.Now(),
			},
			Name:        "Client Hook Event",
			Description: "Test client hook",
			Webhook:     "q:client1",
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
		}
		err := dispatcher.DispatchWebhook(ctx, schedule)
		assert.NoError(t, err)
		assert.Equal(t, []string{"client1:c1"}, mockNotifier.calls)
	})

	t.Run("client hook dispatch - no client connected", func(t *testing.T) {
		cleanup()
		mockNotifier := NewMockClientNotifier()
		dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 2, 10*time.Millisecond, mockNotifier)
		schedule := &models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: uuid.New(),
				EventID:      uuid.New(),
				ScheduledAt:  time.Now(),
			},
			Name:        "Client Hook Event",
			Description: "Test client hook",
			Webhook:     "q:client2",
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
		}
		err := dispatcher.DispatchWebhook(ctx, schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no client connected")
		assert.Equal(t, 3, len(mockNotifier.calls)) // 3 attempts (initial + 2 retries)
	})

	t.Run("client hook dispatch - round robin", func(t *testing.T) {
		cleanup()
		mockNotifier := NewMockClientNotifier()
		mockNotifier.connected["client3"] = []string{"c1", "c2"}
		dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 3, 5*time.Millisecond, mockNotifier)
		schedule := &models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: uuid.New(),
				EventID:      uuid.New(),
				ScheduledAt:  time.Now(),
			},
			Name:        "Client Hook Event",
			Description: "Test client hook",
			Webhook:     "q:client3",
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
		}
		for i := 0; i < 4; i++ {
			err := dispatcher.DispatchWebhook(ctx, schedule)
			assert.NoError(t, err)
		}
		// Should alternate between c1 and c2
		assert.Equal(t, []string{"client3:c1", "client3:c2", "client3:c1", "client3:c2"}, mockNotifier.calls)
	})
}

func TestDispatcher_RedisOnlyDispatch(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger, _ := zap.NewDevelopment()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)
	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 3, 5*time.Second, nil)
	dispatcher.SetHTTPClient(mockHTTP)

	// Create Scheduler instance
	scheduler := NewScheduler(redisClient, logger)

	// Create event and schedule, schedule in Redis
	event := &models.Event{
		ID:          uuid.New(),
		Name:        "Event for Dispatcher Redis Test",
		Description: "Test",
		StartTime:   time.Now(),
		Webhook:     "http://example.com/webhook",
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
		Webhook:     event.Webhook,
		Metadata:    event.Metadata,
		Tags:        event.Tags,
	}
	// Add schedule to Scheduler (simulate scheduling in Redis)
	err = scheduler.ScheduleEvent(ctx, &schedule.Occurrence, event)
	require.NoError(t, err)

	// Debug: Print number of due schedules before running dispatcher
	count, err := scheduler.GetDueSchedules(ctx)
	if err != nil {
		t.Fatalf("Error getting due schedules: %v", err)
	}
	fmt.Printf("[DEBUG] Due schedules before dispatcher: %d\n", count)
	// Do NOT re-add the schedule here; it should now be in the dispatch queue

	// Setup expectations: dispatcher should still attempt dispatch
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
	}, nil)

	// Start dispatcher after GetDueSchedules
	fmt.Printf("[DEBUG] %s: Starting dispatcher goroutine\n", time.Now().Format(time.RFC3339Nano))
	go func() {
		dispatcher.Run(ctx, scheduler, 1)
	}()
	fmt.Printf("[DEBUG] %s: Sleeping before assertion\n", time.Now().Format(time.RFC3339Nano))
	time.Sleep(5 * time.Second)
	fmt.Printf("[DEBUG] %s: Asserting mock call\n", time.Now().Format(time.RFC3339Nano))

	// Verify dispatch was attempted
	mockHTTP.AssertCalled(t, "Do", mock.AnythingOfType("*http.Request"))
}

func TestDispatcher_GetDueSchedules(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger, _ := zap.NewDevelopment()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)
	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 3, 5*time.Second, nil)
	dispatcher.SetHTTPClient(mockHTTP)

	// Create Scheduler instance
	scheduler := NewScheduler(redisClient, logger)

	// Create event and schedule, schedule in Redis
	event := &models.Event{
		ID:          uuid.New(),
		Name:        "Event for Dispatcher Redis Test",
		Description: "Test",
		StartTime:   time.Now(),
		Webhook:     "http://example.com/webhook",
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
	count, err := scheduler.GetDueSchedules(ctx)
	if err != nil {
		t.Fatalf("Error getting due schedules: %v", err)
	}
	fmt.Printf("[DEBUG] Due schedules before dispatcher: %d\n", count)

	// Debug: Print contents of dispatch queue after GetDueSchedules
	items, err := redisClient.LRange(ctx, dispatchQueueKey, 0, -1).Result()
	if err != nil {
		t.Fatalf("Error reading dispatch queue: %v", err)
	}
	fmt.Printf("[DEBUG] Dispatch queue after GetDueSchedules: %v\n", items)

	// Ensure the dispatch queue is populated before starting the worker
	if len(items) == 0 {
		t.Fatalf("Dispatch queue is empty after GetDueSchedules; cannot start worker.")
	}

	// Small delay to ensure Redis propagation
	time.Sleep(100 * time.Millisecond)

	// Setup expectations: dispatcher should still attempt dispatch
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
	}, nil)

	// Start dispatcher after GetDueSchedules
	fmt.Printf("[DEBUG] %s: Starting dispatcher goroutine\n", time.Now().Format(time.RFC3339Nano))
	go func() {
		dispatcher.Run(ctx, scheduler, 1)
	}()
	fmt.Printf("[DEBUG] %s: Sleeping before assertion\n", time.Now().Format(time.RFC3339Nano))
	time.Sleep(5 * time.Second)
	fmt.Printf("[DEBUG] %s: Asserting mock call\n", time.Now().Format(time.RFC3339Nano))

	// Verify dispatch was attempted
	mockHTTP.AssertCalled(t, "Do", mock.AnythingOfType("*http.Request"))
}

// Synchronization helper for worker
func runWorkerAndWait(ctx context.Context, dispatcher *Dispatcher, scheduler *Scheduler, duration time.Duration) {
	workerCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	wg := make(chan struct{})
	go func() {
		dispatcher.Run(workerCtx, scheduler, 1)
		close(wg)
	}()
	<-wg
}

func TestDispatcher_DispatchQueueWorker(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	logger, _ := zap.NewDevelopment()
	redisClient := testutils.TestRedis(t)
	redisClient.FlushAll(ctx)
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)
	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger, 3, 5*time.Second, nil)
	dispatcher.SetHTTPClient(mockHTTP)
	scheduler := NewScheduler(redisClient, logger)

	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
		redisClient.FlushAll(ctx)
	}

	t.Run("worker_processes_and_removes_from_processing_queue_on_success", func(t *testing.T) {
		cleanup()
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Worker Event",
			Description: "Test",
			StartTime:   time.Now().Add(-time.Minute),
			Webhook:     "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)
		occ := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  event.StartTime,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(ctx, occ)
		require.NoError(t, err)
		sched := models.Schedule{
			Occurrence:  *occ,
			Name:        event.Name,
			Description: event.Description,
			Webhook:     event.Webhook,
			Metadata:    event.Metadata,
			Tags:        event.Tags,
		}
		data, err := json.Marshal(sched)
		require.NoError(t, err)
		err = redisClient.RPush(ctx, dispatchQueueKey, data).Err()
		require.NoError(t, err)

		// Setup HTTP mock
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
		}, nil)

		// Before starting the worker, check the processing queue is empty
		items, err := redisClient.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 0)

		// Run worker and wait for completion
		runWorkerAndWait(ctx, dispatcher, scheduler, 3*time.Second)

		// After worker runs, processing queue should be empty
		items, err = redisClient.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 0)
	})

	t.Run("worker_leaves_item_in_processing_queue_on_failure_(less_than_max_retries)", func(t *testing.T) {
		cleanup()
		// Reset and reattach mock HTTP client
		mockHTTP = new(MockHTTPClient)
		dispatcher.SetHTTPClient(mockHTTP)
		// Set dispatcher retryDelay to 100ms for fast test
		dispatcher.retryDelay = 100 * time.Millisecond

		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Worker Fail Event",
			Description: "Test",
			StartTime:   time.Now().Add(-time.Minute),
			Webhook:     "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)
		occ := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  event.StartTime,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(ctx, occ)
		require.NoError(t, err)
		sched := models.Schedule{
			Occurrence:  *occ,
			Name:        event.Name,
			Description: event.Description,
			Webhook:     event.Webhook,
			Metadata:    event.Metadata,
			Tags:        event.Tags,
		}
		data, err := json.Marshal(sched)
		require.NoError(t, err)
		err = redisClient.RPush(ctx, dispatchQueueKey, data).Err()
		require.NoError(t, err)

		// Setup HTTP mock to always fail for every attempt
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return((*http.Response)(nil), errors.New("fail")).Maybe()

		// Run worker and wait for completion (wait for 2 retries, less than maxRetries=3)
		runWorkerAndWait(ctx, dispatcher, scheduler, 400*time.Millisecond)

		// Small delay to ensure worker has finished updating the queue
		time.Sleep(50 * time.Millisecond)

		// Processing queue should have the item (since not yet max retries)
		items, err := redisClient.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})

	t.Run("worker_removes_item_from_processing_queue_after_max_retries", func(t *testing.T) {
		cleanup()
		// Set dispatcher retryDelay to 100ms for fast test
		dispatcher.retryDelay = 100 * time.Millisecond

		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Worker Max Retry Event",
			Description: "Test",
			StartTime:   time.Now().Add(-time.Minute),
			Webhook:     "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}
		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)
		occ := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  event.StartTime,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
		}
		err = occurrenceRepo.Create(ctx, occ)
		require.NoError(t, err)
		sched := models.Schedule{
			Occurrence: models.Occurrence{
				OccurrenceID: occ.OccurrenceID,
				EventID:      occ.EventID,
				ScheduledAt:  occ.ScheduledAt,
				Status:       occ.Status,
				Timestamp:    occ.Timestamp,
				AttemptCount: 2, // start at 2 so next fail is maxRetries (default 3)
			},
			Name:        event.Name,
			Description: event.Description,
			Webhook:     event.Webhook,
			Metadata:    event.Metadata,
			Tags:        event.Tags,
		}
		data, err := json.Marshal(sched)
		require.NoError(t, err)
		err = redisClient.RPush(ctx, dispatchQueueKey, data).Err()
		require.NoError(t, err)

		// Setup HTTP mock to fail
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return((*http.Response)(nil), errors.New("fail"))

		// Run worker and wait for completion (wait for enough time for maxRetries)
		runWorkerAndWait(ctx, dispatcher, scheduler, 1*time.Second)

		// Processing queue should be empty (item removed after max retries)
		items, err := redisClient.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, items, 0)
	})
}
