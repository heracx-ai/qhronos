package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/services"
)

const (
	maxRetries = 3
	retryDelay = 5 * time.Minute
)

type Dispatcher struct {
	client      *http.Client
	eventRepo   *repository.EventRepository
	repo        *repository.OccurrenceRepository
	hmacService *services.HMACService
}

func NewDispatcher(eventRepo *repository.EventRepository, repo *repository.OccurrenceRepository, hmacService *services.HMACService) *Dispatcher {
	return &Dispatcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		eventRepo:   eventRepo,
		repo:        repo,
		hmacService: hmacService,
	}
}

// DispatchWebhook sends a webhook request and handles retries
func (d *Dispatcher) DispatchWebhook(ctx context.Context, occurrence *models.Occurrence, event *models.Event) error {
	// Create request body
	body := map[string]interface{}{
		"event_id":     event.ID,
		"occurrence_id": occurrence.ID,
		"scheduled_at": occurrence.ScheduledAt,
		"metadata":     event.Metadata,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook body: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", event.WebhookURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Sign the request
	var secret string
	if event.HMACSecret != nil {
		secret = *event.HMACSecret
	}
	signature := d.hmacService.SignPayload(jsonBody, secret)
	req.Header.Set("X-Qhronos-Signature", signature)

	// Send request
	resp, err := d.client.Do(req)
	if err != nil {
		return d.handleError(ctx, occurrence, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return d.handleSuccess(ctx, occurrence)
	}

	return d.handleError(ctx, occurrence, fmt.Errorf("webhook returned status %d", resp.StatusCode))
}

// handleSuccess updates the occurrence status to dispatched
func (d *Dispatcher) handleSuccess(ctx context.Context, occurrence *models.Occurrence) error {
	return d.repo.UpdateStatus(ctx, occurrence.ID, models.OccurrenceStatusDispatched)
}

// handleError handles webhook delivery errors and retries
func (d *Dispatcher) handleError(ctx context.Context, occurrence *models.Occurrence, err error) error {
	// Update attempt count
	occurrence.AttemptCount++
	now := time.Now()
	occurrence.LastAttempt = &now

	// Check if we should retry
	if occurrence.AttemptCount < maxRetries {
		// Update status to pending for retry
		if err := d.repo.UpdateStatus(ctx, occurrence.ID, models.OccurrenceStatusPending); err != nil {
			return fmt.Errorf("failed to update occurrence status: %w", err)
		}

		// Schedule retry
		occurrence.ScheduledAt = time.Now().Add(retryDelay)
		return nil
	}

	// Max retries reached, mark as failed
	return d.repo.UpdateStatus(ctx, occurrence.ID, models.OccurrenceStatusFailed)
}

// Run processes due events and dispatches webhooks
func (d *Dispatcher) Run(ctx context.Context, scheduler *Scheduler) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Get due events
			occurrences, err := scheduler.GetDueEvents(ctx, 100)
			if err != nil {
				fmt.Printf("Error getting due events: %v\n", err)
				continue
			}

			// Process each occurrence
			for _, occurrence := range occurrences {
				// Get the associated event
				event, err := d.eventRepo.GetByID(ctx, occurrence.EventID)
				if err != nil {
					fmt.Printf("Error getting event %s: %v\n", occurrence.EventID, err)
					continue
				}

				// Dispatch webhook
				if err := d.DispatchWebhook(ctx, occurrence, event); err != nil {
					fmt.Printf("Error dispatching webhook for occurrence %s: %v\n", occurrence.ID, err)
					continue
				}

				// Remove from scheduler
				if err := scheduler.RemoveScheduledEvent(ctx, occurrence); err != nil {
					fmt.Printf("Error removing scheduled event %s: %v\n", occurrence.ID, err)
				}
			}
		}
	}
} 