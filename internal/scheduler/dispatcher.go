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

// DispatchWebhook sends a webhook request and handles a single attempt (no in-process retry)
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
		err = d.clientNotifier.DispatchToClient(clientID, jsonPayload)
		finalStatus := models.OccurrenceStatusCompleted
		if err != nil {
			finalStatus = models.OccurrenceStatusFailed
		}
		logOccurrence := &models.Occurrence{
			OccurrenceID: sched.OccurrenceID,
			EventID:      sched.EventID,
			ScheduledAt:  sched.ScheduledAt,
			Status:       finalStatus,
			AttemptCount: sched.AttemptCount,
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: fmt.Sprintf("client hook dispatch: %v", err),
		}
		_ = d.occurrenceRepo.Create(ctx, logOccurrence)
		return err
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
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %w", err)
	}
	baseReq, err := http.NewRequestWithContext(ctx, "POST", sched.Webhook, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	baseReq.Header.Set("Content-Type", "application/json")
	baseReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(jsonPayload)))
	if d.hmacService != nil {
		secret := ""
		signature := d.hmacService.SignPayload(jsonPayload, secret)
		baseReq.Header.Set("X-Qhronos-Signature", signature)
	}
	// Only one attempt per call
	attemptCount := sched.AttemptCount
	if attemptCount == 0 {
		attemptCount = 1
	} else {
		attemptCount++
	}
	req := baseReq.Clone(ctx)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(jsonPayload))
	var finalStatus models.OccurrenceStatus
	var statusCode int
	var responseBody string
	var errorMessage string
	resp, err := d.client.Do(req)
	if err != nil {
		finalStatus = models.OccurrenceStatusFailed
		statusCode = 0
		responseBody = ""
		errorMessage = err.Error()
	} else if resp == nil {
		finalStatus = models.OccurrenceStatusFailed
		statusCode = 0
		responseBody = ""
		errorMessage = "empty response from server"
	} else {
		defer func() {
			if resp.Body != nil {
				resp.Body.Close()
			}
		}()
		statusCode = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			finalStatus = models.OccurrenceStatusCompleted
			responseBody = ""
			errorMessage = ""
		} else {
			finalStatus = models.OccurrenceStatusFailed
			responseBody = ""
			errorMessage = fmt.Sprintf("received non-2xx status code: %d", resp.StatusCode)
		}
	}
	logOccurrence := &models.Occurrence{
		OccurrenceID: sched.OccurrenceID,
		EventID:      sched.EventID,
		ScheduledAt:  sched.ScheduledAt,
		Status:       finalStatus,
		AttemptCount: attemptCount,
		Timestamp:    time.Now(),
		StatusCode:   statusCode,
		ResponseBody: responseBody,
		ErrorMessage: errorMessage,
	}
	_ = d.occurrenceRepo.Create(ctx, logOccurrence)
	// Auto-inactivate one-time events after dispatch (success or max retries)
	event, err := d.eventRepo.GetByID(ctx, sched.EventID)
	if event == nil {
		return fmt.Errorf("event not found: %s", sched.EventID)
	}
	if err == nil && event.Schedule == nil && event.Status == "active" {
		event.Status = "inactive"
		_ = d.eventRepo.Update(ctx, event)
	}
	if finalStatus == models.OccurrenceStatusCompleted {
		return nil
	}
	return fmt.Errorf("dispatch failed: %s", errorMessage)
}

const retryQueueKey = "retry:queue"

// Run starts a pool of dispatcher workers that process the dispatch queue
func (d *Dispatcher) Run(ctx context.Context, scheduler *Scheduler, workerCount int) error {
	d.logger.Info("Starting dispatcher worker pool", zap.Int("worker_count", workerCount))

	// Lua script for atomic move from retry queue to dispatch queue
	retryLua := `
local due = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
for i, v in ipairs(due) do
  redis.call('RPUSH', KEYS[2], v)
  redis.call('ZREM', KEYS[1], v)
end
return due
`

	// Start retry poller
	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pollerCtx.Done():
				return
			case <-ticker.C:
				now := fmt.Sprintf("%f", float64(time.Now().Unix()))
				// Use Lua script for atomic move
				res, err := scheduler.redis.Eval(ctx, retryLua, []string{retryQueueKey, dispatchQueueKey}, now).Result()
				if err != nil {
					d.logger.Error("[RETRY POLLER] Lua script failed", zap.Error(err))
					continue
				}
				if arr, ok := res.([]interface{}); ok && len(arr) > 0 {
					d.logger.Debug("[RETRY POLLER] Moved items from retry queue to dispatch queue", zap.Int("count", len(arr)))
				}
			}
		}
	}()

	workerFn := func(workerID int) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				itemStart := time.Now()
				d.logger.Debug("[DISPATCHER] Worker waiting for item", zap.Int("worker_id", workerID), zap.Time("ts", itemStart))
				// Pop from dispatch queue (no processing queue)
				popStart := time.Now()
				data, err := scheduler.redis.BRPop(ctx, 5*time.Second, dispatchQueueKey).Result()
				popEnd := time.Now()
				d.logger.Debug("[DISPATCHER] BRPop duration", zap.Int("worker_id", workerID), zap.Duration("duration", popEnd.Sub(popStart)), zap.Error(err))
				if err == redis.Nil {
					d.logger.Debug("[DISPATCHER] No item found, continuing", zap.Int("worker_id", workerID), zap.Time("ts", time.Now()))
					continue // No item, keep waiting
				} else if err != nil {
					d.logger.Error("Worker failed to BRPOP", zap.Int("worker_id", workerID), zap.Error(err))
					continue
				}
				// BRPop returns [queue, value]
				item := data[1]
				unmarshalStart := time.Now()
				var sched models.Schedule
				if err := json.Unmarshal([]byte(item), &sched); err != nil {
					d.logger.Error("Worker failed to unmarshal schedule", zap.Int("worker_id", workerID), zap.Error(err), zap.String("data", item))
					continue
				}
				unmarshalEnd := time.Now()
				d.logger.Debug("[DISPATCHER] Unmarshal duration", zap.Int("worker_id", workerID), zap.Duration("duration", unmarshalEnd.Sub(unmarshalStart)))
				d.logger.Debug("[DISPATCHER] Worker unmarshalled schedule", zap.Int("worker_id", workerID), zap.Any("schedule", sched))
				d.logger.Debug("[DISPATCHER] Worker dispatching webhook", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()))

				// Track and increment attempt count
				if sched.AttemptCount == 0 {
					sched.AttemptCount = 1
				} else {
					sched.AttemptCount++
				}

				dispatchStart := time.Now()
				err = d.DispatchWebhook(ctx, &sched)
				dispatchEnd := time.Now()
				d.logger.Debug("[DISPATCHER] DispatchWebhook duration", zap.Int("worker_id", workerID), zap.Duration("duration", dispatchEnd.Sub(dispatchStart)), zap.Error(err))

				if err != nil {
					d.logger.Error("Worker failed to dispatch webhook", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()), zap.Error(err), zap.Int("attempt_count", sched.AttemptCount))
					if sched.AttemptCount >= d.maxRetries {
						d.logger.Debug("[DISPATCHER] Max retries exceeded, dropping item", zap.Int("worker_id", workerID), zap.Any("schedule", sched))
						// No further action needed, item is dropped
					} else {
						zaddStart := time.Now()
						nextRetry := time.Now().Add(d.retryDelay).Unix()
						updatedData, _ := json.Marshal(sched)
						d.logger.Debug("[DISPATCHER] Before ZAdd to retry queue", zap.Int("worker_id", workerID), zap.Time("ts", zaddStart))
						err := scheduler.redis.ZAdd(ctx, retryQueueKey, redis.Z{
							Score:  float64(nextRetry),
							Member: updatedData,
						}).Err()
						zaddEnd := time.Now()
						d.logger.Debug("[DISPATCHER] After ZAdd to retry queue", zap.Int("worker_id", workerID), zap.Time("ts", zaddEnd), zap.Duration("duration", zaddEnd.Sub(zaddStart)), zap.Error(err))
						if err != nil {
							d.logger.Error("[DISPATCHER] Failed to add to retry queue", zap.Int("worker_id", workerID), zap.Error(err))
						}
						d.logger.Debug("[DISPATCHER] Item moved to retry queue", zap.Int("worker_id", workerID), zap.Any("schedule", sched), zap.Int64("next_retry", nextRetry))
					}
					itemEnd := time.Now()
					d.logger.Debug("[DISPATCHER] Total time for item", zap.Int("worker_id", workerID), zap.Duration("duration", itemEnd.Sub(itemStart)))
					continue
				}
				d.logger.Debug("[DISPATCHER] Worker successfully dispatched webhook", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()))
				itemEnd := time.Now()
				d.logger.Debug("[DISPATCHER] Total time for item", zap.Int("worker_id", workerID), zap.Duration("duration", itemEnd.Sub(itemStart)))
			}
		}
	}
	for i := 0; i < workerCount; i++ {
		go workerFn(i)
	}
	<-ctx.Done()
	pollerCancel()
	d.logger.Info("Dispatcher worker pool shutting down")
	return ctx.Err()
}

func timePtr(t time.Time) *time.Time {
	return &t
}
