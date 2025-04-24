package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/services"
	"go.uber.org/zap"
)

type Dispatcher struct {
	eventRepo      *repository.EventRepository
	occurrenceRepo *repository.OccurrenceRepository
	hmacService    *services.HMACService
	client         HTTPClient
	maxRetries     int
	retryDelay     time.Duration
	logger         *zap.Logger
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

func NewDispatcher(eventRepo *repository.EventRepository, occurrenceRepo *repository.OccurrenceRepository, hmacService *services.HMACService, logger *zap.Logger) *Dispatcher {
	return &Dispatcher{
		eventRepo:      eventRepo,
		occurrenceRepo: occurrenceRepo,
		hmacService:    hmacService,
		client:         &DefaultHTTPClient{client: &http.Client{Timeout: 10 * time.Second}},
		maxRetries:     3,
		retryDelay:     5 * time.Second,
		logger:         logger,
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
	err = d.retryWithBackoff(ctx, func() error {
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

	return err
}

// Run processes due schedules and dispatches webhooks
func (d *Dispatcher) Run(ctx context.Context, scheduler *Scheduler) error {
	d.logger.Info("Starting dispatcher")
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("Dispatcher shutting down")
			return ctx.Err()
		case <-ticker.C:
			// Get due schedules
			schedules, err := scheduler.GetDueSchedules(ctx)
			if err != nil {
				d.logger.Error("Error getting due schedules", zap.Error(err))
				continue
			}

			// Process each schedule
			for _, sched := range schedules {
				if err := d.DispatchWebhook(ctx, sched); err != nil {
					d.logger.Error("Error dispatching webhook",
						zap.String("occurrence_id", sched.OccurrenceID.String()),
						zap.Error(err))
					continue
				}
			}
		}
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
