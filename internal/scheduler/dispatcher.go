package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ClientNotifier abstracts WebSocket client dispatch
// (or use your preferred mock generator)
//
//go:generate mockery --name=ClientNotifier --output=../mocks --case=underscore
type ClientNotifier interface {
	DispatchToClient(clientID string, payload []byte) error
}

type Dispatcher struct {
	eventRepo      *repository.EventRepository
	occurrenceRepo *repository.OccurrenceRepository
	hmacService    *services.HMACService
	client         HTTPClient
	maxRetries     int
	retryDelay     time.Duration
	logger         *zap.Logger
	clientNotifier ClientNotifier // optional, for q: webhooks
}

// HTTPClient interface for mocking HTTP requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient implements HTTPClient using http.Client
type DefaultHTTPClient struct {
	client *http.Client
}

func (d *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

func NewDispatcher(eventRepo *repository.EventRepository, occurrenceRepo *repository.OccurrenceRepository, hmacService *services.HMACService, logger *zap.Logger, maxRetries int, retryDelay time.Duration, clientNotifier ClientNotifier) *Dispatcher {
	return &Dispatcher{
		eventRepo:      eventRepo,
		occurrenceRepo: occurrenceRepo,
		hmacService:    hmacService,
		client:         &DefaultHTTPClient{client: &http.Client{Timeout: 10 * time.Second}},
		maxRetries:     maxRetries,
		retryDelay:     retryDelay,
		logger:         logger,
		clientNotifier: clientNotifier,
	}
}

// SetHTTPClient allows setting a custom HTTP client (used for testing)
func (d *Dispatcher) SetHTTPClient(client HTTPClient) {
	d.client = client
}

// retryWithBackoff executes a function with retries and backoff
func (d *Dispatcher) retryWithBackoff(ctx context.Context, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt == d.maxRetries {
			return lastErr
		}

		time.Sleep(d.retryDelay)
	}
	return lastErr
}

// DispatchWebhook sends a webhook request and handles retries using a Schedule object
func (d *Dispatcher) DispatchWebhook(ctx context.Context, sched *models.Schedule) error {
	if strings.HasPrefix(sched.Webhook, "q:") {
		if d.clientNotifier == nil {
			return fmt.Errorf("client notifier not configured for q: webhooks")
		}
		clientID := strings.TrimPrefix(sched.Webhook, "q:")
		payload := map[string]interface{}{
			"event_id":      sched.EventID,
			"occurrence_id": sched.OccurrenceID,
			"name":          sched.Name,
			"description":   sched.Description,
			"scheduled_at":  sched.ScheduledAt,
			"metadata":      sched.Metadata,
		}
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshaling payload: %w", err)
		}
		var dispatchErr error
		dispatchErr = d.retryWithBackoff(ctx, func() error {
			return d.clientNotifier.DispatchToClient(clientID, jsonPayload)
		})
		// Log occurrence as before (reuse code)
		// ... (copy occurrence logging from HTTP path, set status based on dispatchErr) ...
		finalStatus := models.OccurrenceStatusCompleted
		if dispatchErr != nil {
			finalStatus = models.OccurrenceStatusFailed
		}
		logOccurrence := &models.Occurrence{
			OccurrenceID: sched.OccurrenceID,
			EventID:      sched.EventID,
			ScheduledAt:  sched.ScheduledAt,
			Status:       finalStatus,
			AttemptCount: 1, // or track attempts if needed
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: fmt.Sprintf("client hook dispatch: %v", dispatchErr),
		}
		_ = d.occurrenceRepo.Create(ctx, logOccurrence)
		return dispatchErr
	}
	// Prepare webhook payload with rich information
	payload := map[string]interface{}{
		"event_id":      sched.EventID,
		"occurrence_id": sched.OccurrenceID,
		"name":          sched.Name,
		"description":   sched.Description,
		"scheduled_at":  sched.ScheduledAt,
		"metadata":      sched.Metadata,
	}

	// Convert payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %w", err)
	}

	// Create base request (will be cloned for each attempt)
	baseReq, err := http.NewRequestWithContext(ctx, "POST", sched.Webhook, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	baseReq.Header.Set("Content-Type", "application/json")
	baseReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonPayload)))

	// Sign request if HMAC is enabled (not supported if secret is not present)
	if d.hmacService != nil {
		// No HMACSecret in Schedule, so skip or use default
		secret := ""
		signature := d.hmacService.SignPayload(jsonPayload, secret)
		baseReq.Header.Set("X-Qhronos-Signature", signature)
	}

	// Track attempts
	attemptCount := 0
	var lastAttempt time.Time
	var finalStatus models.OccurrenceStatus
	var statusCode int
	var responseBody string
	var errorMessage string

	// Execute webhook request with retries
	dispatchErr := d.retryWithBackoff(ctx, func() error {
		attemptCount++
		lastAttempt = time.Now()
		// Clone the base request and set the body for this attempt
		req := baseReq.Clone(ctx)
		req.Body = ioutil.NopCloser(bytes.NewBuffer(jsonPayload))

		resp, err := d.client.Do(req)
		if err != nil {
			finalStatus = models.OccurrenceStatusFailed
			statusCode = 0
			responseBody = ""
			errorMessage = err.Error()
			return fmt.Errorf("webhook request failed: %w", err)
		}

		// Handle nil response
		if resp == nil {
			finalStatus = models.OccurrenceStatusFailed
			statusCode = 0
			responseBody = ""
			errorMessage = "empty response from server"
			return fmt.Errorf("empty response from server")
		}

		// Ensure response body is closed
		defer func() {
			if resp.Body != nil {
				resp.Body.Close()
			}
		}()

		// Check response status
		statusCode = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			finalStatus = models.OccurrenceStatusCompleted
			responseBody = ""
			errorMessage = ""
			return nil
		}

		// Non-2xx status code
		finalStatus = models.OccurrenceStatusFailed
		responseBody = ""
		errorMessage = fmt.Sprintf("received non-2xx status code: %d", resp.StatusCode)
		return fmt.Errorf("received non-2xx status code: %d", resp.StatusCode)
	})

	// Log the result to Postgres as a new occurrence record (append-only, for history)
	logOccurrence := &models.Occurrence{
		OccurrenceID: sched.OccurrenceID,
		EventID:      sched.EventID,
		ScheduledAt:  sched.ScheduledAt,
		Status:       finalStatus,
		AttemptCount: attemptCount,
		Timestamp:    lastAttempt,
		StatusCode:   statusCode,
		ResponseBody: responseBody,
		ErrorMessage: errorMessage,
	}
	_ = d.occurrenceRepo.Create(ctx, logOccurrence) // Ignore error to avoid blocking delivery

	// Auto-inactivate one-time events after dispatch (success or max retries)
	event, err := d.eventRepo.GetByID(ctx, sched.EventID)
	if event == nil {
		return fmt.Errorf("event not found: %s", sched.EventID)
	}
	if err == nil && event.Schedule == nil && event.Status == "active" {
		event.Status = "inactive"
		_ = d.eventRepo.Update(ctx, event)
	}

	return dispatchErr
}

const dispatchProcessingKey = "dispatch:processing"

// Run starts a pool of dispatcher workers that process the dispatch queue
func (d *Dispatcher) Run(ctx context.Context, scheduler *Scheduler, workerCount int) error {
	d.logger.Info("Starting dispatcher worker pool", zap.Int("worker_count", workerCount))
	workerFn := func(workerID int) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				d.logger.Debug("[DISPATCHER] Worker waiting for item", zap.Int("worker_id", workerID))
				// Atomically move from queue to processing
				data, err := scheduler.redis.BRPopLPush(ctx, dispatchQueueKey, dispatchProcessingKey, 5*time.Second).Result()
				if err == redis.Nil {
					d.logger.Debug("[DISPATCHER] No item found, continuing", zap.Int("worker_id", workerID))
					continue // No item, keep waiting
				} else if err != nil {
					d.logger.Error("Worker failed to BRPOPLPUSH", zap.Int("worker_id", workerID), zap.Error(err))
					continue
				}
				d.logger.Debug("[DISPATCHER] Worker got item from queue", zap.Int("worker_id", workerID), zap.String("data", data))
				var sched models.Schedule
				if err := json.Unmarshal([]byte(data), &sched); err != nil {
					d.logger.Error("Worker failed to unmarshal schedule", zap.Int("worker_id", workerID), zap.Error(err), zap.String("data", data))
					// Remove the bad item from processing
					_ = scheduler.redis.LRem(ctx, dispatchProcessingKey, 1, data).Err()
					continue
				}
				d.logger.Debug("[DISPATCHER] Worker unmarshalled schedule", zap.Int("worker_id", workerID), zap.Any("schedule", sched))
				d.logger.Debug("[DISPATCHER] Worker dispatching webhook", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()))

				// Track and increment attempt count
				if sched.AttemptCount == 0 {
					sched.AttemptCount = 1
				} else {
					sched.AttemptCount++
				}

				// Attempt dispatch
				err = d.DispatchWebhook(ctx, &sched)
				if err != nil {
					d.logger.Error("Worker failed to dispatch webhook", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()), zap.Error(err), zap.Int("attempt_count", sched.AttemptCount))
					// Always log failed occurrence (already done in DispatchWebhook)
					if sched.AttemptCount >= d.maxRetries {
						// Remove from processing queue, do not push to dead letter queue
						queueBefore, _ := scheduler.redis.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
						d.logger.Debug("[DISPATCHER] Max retries exceeded, removing from processing queue", zap.Int("worker_id", workerID), zap.Any("queue", queueBefore), zap.String("removing", data))
						// Use non-cancellable context for removal
						_ = scheduler.redis.LRem(context.Background(), dispatchProcessingKey, 1, data).Err()
						queueAfter, _ := scheduler.redis.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
						d.logger.Debug("[DISPATCHER] Processing queue after LRem (max retries)", zap.Int("worker_id", workerID), zap.Any("queue", queueAfter))
					} else {
						// Debug: log queue state before LSet
						queueBeforeLSet, _ := scheduler.redis.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
						d.logger.Debug("[DISPATCHER] Processing queue before LSet (increment attempt count)", zap.Int("worker_id", workerID), zap.Any("queue", queueBeforeLSet))
						// Update the item in the processing queue with incremented attempt count
						updatedData, _ := json.Marshal(sched)
						lsetErr := scheduler.redis.LSet(ctx, dispatchProcessingKey, 0, updatedData).Err() // LSet index 0: most recent item
						if lsetErr != nil {
							d.logger.Error("[DISPATCHER] LSet failed when incrementing attempt count", zap.Int("worker_id", workerID), zap.Error(lsetErr))
						}
						// Debug: log queue state after LSet
						queueAfterLSet, _ := scheduler.redis.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
						d.logger.Debug("[DISPATCHER] Processing queue after LSet (increment attempt count)", zap.Int("worker_id", workerID), zap.Any("queue", queueAfterLSet))
					}
					continue
				}
				d.logger.Debug("[DISPATCHER] Worker successfully dispatched webhook", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()))
				// On success, remove from processing queue using the exact value popped (raw JSON string)
				queueBefore, _ := scheduler.redis.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
				d.logger.Debug("[DISPATCHER] Processing queue before LRem", zap.Int("worker_id", workerID), zap.Any("queue", queueBefore), zap.String("removing", data))
				// Use non-cancellable context for removal
				_ = scheduler.redis.LRem(context.Background(), dispatchProcessingKey, 1, data).Err()
				queueAfter, _ := scheduler.redis.LRange(ctx, dispatchProcessingKey, 0, -1).Result()
				d.logger.Debug("[DISPATCHER] Processing queue after LRem", zap.Int("worker_id", workerID), zap.Any("queue", queueAfter))
			}
		}
	}
	for i := 0; i < workerCount; i++ {
		go workerFn(i)
	}
	<-ctx.Done()
	d.logger.Info("Dispatcher worker pool shutting down")
	return ctx.Err()
}

func timePtr(t time.Time) *time.Time {
	return &t
}
