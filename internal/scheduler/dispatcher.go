package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/feedloop/qhronos/internal/actions"
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
	clientNotifier ClientNotifier          // optional, for q: webhooks
	scheduler      *Scheduler              // new field for scheduler
	redisPrefix    string                  // added redisPrefix field
	actionsManager *actions.ActionsManager // new field for actions manager
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

func NewDispatcher(eventRepo *repository.EventRepository, occurrenceRepo *repository.OccurrenceRepository, hmacService *services.HMACService, logger *zap.Logger, maxRetries int, retryDelay time.Duration, clientNotifier ClientNotifier, scheduler *Scheduler, httpClient HTTPClient) *Dispatcher {
	am := actions.NewActionsManager()
	var webhookClient HTTPClient
	if httpClient != nil {
		webhookClient = httpClient
	} else {
		webhookClient = &DefaultHTTPClient{client: &http.Client{Timeout: 10 * time.Second}}
	}
	am.Register(models.ActionTypeWebhook, actions.NewWebhookExecutor(hmacService, webhookClient))
	am.Register(models.ActionTypeWebsocket, actions.NewWebsocketExecutor(clientNotifier))
	am.Register(models.ActionTypeAPICall, actions.NewAPICallExecutor(webhookClient))
	return &Dispatcher{
		eventRepo:      eventRepo,
		occurrenceRepo: occurrenceRepo,
		hmacService:    hmacService,
		client:         webhookClient,
		maxRetries:     maxRetries,
		retryDelay:     retryDelay,
		logger:         logger,
		clientNotifier: clientNotifier,
		scheduler:      scheduler,
		redisPrefix:    scheduler.redisPrefix,
		actionsManager: am,
	}
}

// SetHTTPClient allows setting a custom HTTP client (used for testing)
func (d *Dispatcher) SetHTTPClient(client HTTPClient) {
	d.client = client
}

// DispatchAction sends a webhook request or dispatches a websocket message based on the event's action type.
func (d *Dispatcher) DispatchAction(ctx context.Context, sched *models.Schedule) error {
	// Fetch the full event details, as sched from Redis might be minimal
	event, err := d.eventRepo.GetByID(ctx, sched.EventID)
	if err != nil {
		d.logger.Error("Failed to get event for dispatch", zap.String("event_id", sched.EventID.String()), zap.String("occurrence_id", sched.OccurrenceID.String()), zap.Error(err))
		// Log occurrence as failed if event cannot be fetched
		logErrOccurrence(ctx, d.occurrenceRepo, sched, 0, "", fmt.Sprintf("event not found for dispatch: %v", err))
		// Defensive: Clean up orphaned schedule if event does not exist (similar to old logic)
		key := fmt.Sprintf("schedule:%s:%d", sched.EventID.String(), sched.ScheduledAt.Unix())
		if d.scheduler != nil && d.scheduler.redis != nil {
			_, _ = d.scheduler.redis.ZRem(ctx, d.scheduler.redisPrefix+ScheduleKey, key).Result()     // Best effort
			_, _ = d.scheduler.redis.HDel(ctx, d.scheduler.redisPrefix+"schedule:data", key).Result() // Best effort
			d.logger.Warn("Orphaned schedule possibly removed after failing to fetch event for dispatch", zap.String("event_id", sched.EventID.String()), zap.String("schedule_key", key))
		}
		return fmt.Errorf("event not found for dispatch: %s, error: %w", sched.EventID, err)
	}
	if event == nil { // Should be covered by err != nil if repo returns sql.ErrNoRows correctly mapped
		d.logger.Error("Event is nil after GetByID for dispatch", zap.String("event_id", sched.EventID.String()), zap.String("occurrence_id", sched.OccurrenceID.String()))
		logErrOccurrence(ctx, d.occurrenceRepo, sched, 0, "", "event is nil after GetByID")
		return fmt.Errorf("event is nil after GetByID: %s", sched.EventID)
	}

	// Ensure Action is present. The repository should have populated it.
	// If not, log a warning. The migration should handle old data.
	if event.Action == nil {
		d.logger.Error("Event action is nil, cannot dispatch", zap.String("event_id", event.ID.String()), zap.String("occurrence_id", sched.OccurrenceID.String()))
		logErrOccurrence(ctx, d.occurrenceRepo, sched, 0, "", "event action is nil")
		return fmt.Errorf("event action is nil for event %s", event.ID)
	}

	// Increment attempt count for the schedule from Redis before dispatching
	// Note: sched.AttemptCount is from Redis and might not reflect the absolute truth if there were prior system crashes.
	// The occurrence table logging will reflect the true attempt for that specific logged dispatch.
	currentAttempt := sched.AttemptCount + 1 // This is the attempt number for *this* dispatch cycle from Redis data.

	var dispatchError error
	var statusCode int
	var responseBody string
	var finalStatus models.OccurrenceStatus

	dispatchError = d.actionsManager.Execute(ctx, event)
	// Optionally, you can enhance ActionsManager.Execute to return statusCode/responseBody if needed in the future.
	// For now, just handle error/success as before.

	// Log Occurrence
	errorMessage := ""
	if dispatchError != nil {
		finalStatus = models.OccurrenceStatusFailed
		errorMessage = dispatchError.Error()
	} else {
		finalStatus = models.OccurrenceStatusCompleted
	}

	logOccurrence := &models.Occurrence{
		OccurrenceID: sched.OccurrenceID,
		EventID:      event.ID,
		ScheduledAt:  sched.ScheduledAt,
		Status:       finalStatus,
		AttemptCount: currentAttempt, // Use the attempt count for this dispatch cycle
		Timestamp:    time.Now(),
		StatusCode:   statusCode,
		ResponseBody: responseBody, // May be empty for websocket
		ErrorMessage: errorMessage,
		StartedAt:    sched.ScheduledAt, // Assign time.Time directly
		CompletedAt:  time.Now(),        // Assign time.Time directly
	}
	if err := d.occurrenceRepo.Create(ctx, logOccurrence); err != nil {
		d.logger.Error("Failed to log occurrence", zap.Error(err), zap.String("occurrence_id", sched.OccurrenceID.String()))
		// Even if logging fails, the original dispatchError determines the return for retry logic.
	}

	// Auto-inactivate one-time events after dispatch (success or max retries - this check is now implicit in worker)
	if event.Schedule == nil && event.Status == models.EventStatusActive && finalStatus == models.OccurrenceStatusCompleted {
		// If it's a one-time event and completed successfully, mark as inactive.
		// Retry logic in Run() handles maxRetries failures.
		event.Status = models.EventStatusInactive
		if err := d.eventRepo.Update(ctx, event); err != nil {
			d.logger.Error("Failed to auto-inactivate event", zap.String("event_id", event.ID.String()), zap.Error(err))
		}
	}

	// Auto-inactivate recurring events if count or until is reached
	if event.Schedule != nil && event.Status == models.EventStatusActive && finalStatus == models.OccurrenceStatusCompleted {
		shouldInactivate := false
		if event.Schedule.Count != nil {
			completedCount, err := d.occurrenceRepo.CountCompletedByEventID(ctx, event.ID)
			if err == nil && completedCount >= *event.Schedule.Count {
				shouldInactivate = true
			}
		}
		if !shouldInactivate && event.Schedule.Until != nil {
			untilTime, err := time.Parse(time.RFC3339, *event.Schedule.Until)
			if err == nil && time.Now().After(untilTime) {
				shouldInactivate = true
			}
		}
		if shouldInactivate {
			event.Status = models.EventStatusInactive
			if err := d.eventRepo.Update(ctx, event); err != nil {
				d.logger.Error("Failed to auto-inactivate recurring event", zap.String("event_id", event.ID.String()), zap.Error(err))
			} else {
				d.logger.Info("Auto-inactivated recurring event", zap.String("event_id", event.ID.String()))
			}
		}
	}

	return dispatchError // This error determines if it goes to retry queue
}

// Helper to log an error occurrence
func logErrOccurrence(ctx context.Context, repo *repository.OccurrenceRepository, sched *models.Schedule, attempt int, respBody, errMsg string) {
	if repo == nil || sched == nil {
		return
	}
	log := &models.Occurrence{
		OccurrenceID: sched.OccurrenceID,
		EventID:      sched.EventID,
		ScheduledAt:  sched.ScheduledAt,
		Status:       models.OccurrenceStatusFailed,
		AttemptCount: attempt,
		Timestamp:    time.Now(),
		ResponseBody: respBody,
		ErrorMessage: errMsg,
	}
	_ = repo.Create(ctx, log) // Best effort logging
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
				res, err := scheduler.redis.Eval(ctx, retryLua, []string{d.scheduler.redisPrefix + retryQueueKey, d.scheduler.redisPrefix + dispatchQueueKey}, now).Result()
				if err != nil && err != redis.Nil { // redis.Nil can happen if script returns empty array but is not an actual error
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
				d.logger.Info("Dispatcher worker shutting down", zap.Int("worker_id", workerID))
				return
			default:
				// Pop from dispatch queue
				data, err := scheduler.redis.BRPop(ctx, 5*time.Second, d.scheduler.redisPrefix+dispatchQueueKey).Result()
				if err == redis.Nil {
					continue // No item, keep waiting
				} else if err != nil {
					d.logger.Error("Worker failed to BRPop from dispatch queue", zap.Int("worker_id", workerID), zap.Error(err))
					time.Sleep(1 * time.Second) // Avoid fast loop on persistent BRPop errors
					continue
				}

				item := data[1] // BRPop returns [queue, value]
				var sched models.Schedule
				if err := json.Unmarshal([]byte(item), &sched); err != nil {
					d.logger.Error("Worker failed to unmarshal schedule from dispatch queue", zap.Int("worker_id", workerID), zap.Error(err), zap.String("data", item))
					continue
				}

				d.logger.Debug("Worker picked up schedule for dispatch", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()))

				// DispatchAction will handle logging the occurrence and fetching the full event.
				// The sched.AttemptCount from Redis is passed to give context on retries for this queue item.
				dispatchErr := d.DispatchAction(ctx, &sched)

				if dispatchErr != nil {
					d.logger.Error("DispatchAction failed",
						zap.Int("worker_id", workerID),
						zap.String("occurrence_id", sched.OccurrenceID.String()),
						zap.Int("redis_attempt_count", sched.AttemptCount+1),
						zap.Error(dispatchErr))

					if sched.AttemptCount+1 >= d.maxRetries {
						d.logger.Warn("Max retries exceeded for occurrence, dropping item",
							zap.Int("worker_id", workerID),
							zap.String("occurrence_id", sched.OccurrenceID.String()),
							zap.Int("attempts", sched.AttemptCount+1))
						// Final failure is logged by DispatchAction itself via logErrOccurrence or by successful creation with failed status.
					} else {
						// Re-queue for retry
						sched.AttemptCount++ // Increment attempt for next try
						updatedData, marshalErr := json.Marshal(sched)
						if marshalErr != nil {
							d.logger.Error("Failed to marshal schedule for retry queue", zap.Error(marshalErr), zap.String("occurrence_id", sched.OccurrenceID.String()))
							continue // Skip re-queueing if marshaling fails
						}
						nextRetryTime := time.Now().Add(d.retryDelay).Unix()
						if zerr := scheduler.redis.ZAdd(ctx, d.scheduler.redisPrefix+retryQueueKey, redis.Z{
							Score:  float64(nextRetryTime),
							Member: updatedData,
						}).Err(); zerr != nil {
							d.logger.Error("Failed to add item to retry queue", zap.Error(zerr), zap.String("occurrence_id", sched.OccurrenceID.String()))
						} else {
							d.logger.Info("Item moved to retry queue", zap.String("occurrence_id", sched.OccurrenceID.String()), zap.Int("next_attempt", sched.AttemptCount))
						}
					}
				} else {
					d.logger.Info("DispatchAction successful", zap.Int("worker_id", workerID), zap.String("occurrence_id", sched.OccurrenceID.String()))
				}
			}
		}
	}
	for i := 0; i < workerCount; i++ {
		go workerFn(i)
	}

	<-ctx.Done()
	pollerCancel()
	d.logger.Info("Dispatcher worker pool shutting down.")
	return nil // Or ctx.Err() if you want to propagate it
}

// timePtr helper, can be removed if not used elsewhere.
func timePtr(t time.Time) *time.Time {
	return &t
}
