package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/google/uuid"
)

type Expander struct {
	scheduler *Scheduler
	repo      *repository.EventRepository
}

func NewExpander(scheduler *Scheduler, repo *repository.EventRepository) *Expander {
	return &Expander{
		scheduler: scheduler,
		repo:      repo,
	}
}

// ExpandRecurringEvents expands recurring events into occurrences
func (e *Expander) ExpandRecurringEvents(ctx context.Context) error {
	// Get all recurring events
	events, err := e.scheduler.GetRecurringEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recurring events: %w", err)
	}

	now := time.Now()
	// Look ahead 24 hours for occurrences
	endTime := now.Add(24 * time.Hour)

	for _, event := range events {
		if event.Schedule == nil {
			continue
		}

		// Get occurrences between now and endTime
		occurrences, err := GetOccurrencesBetween(*event.Schedule, now, endTime)
		if err != nil {
			return fmt.Errorf("failed to get occurrences for event %s: %w", event.ID, err)
		}

		// Create occurrences for each time
		for _, occurrenceTime := range occurrences {
			occurrence := &models.Occurrence{
				ID:           uuid.New(),
				EventID:      event.ID,
				ScheduledAt:  occurrenceTime,
				Status:       models.OccurrenceStatusPending,
				LastAttempt:  nil,
				AttemptCount: 0,
				CreatedAt:    now,
			}

			// Schedule the occurrence
			if err := e.scheduler.ScheduleEvent(ctx, occurrence); err != nil {
				return fmt.Errorf("failed to schedule occurrence for event %s: %w", event.ID, err)
			}
		}
	}

	return nil
}

// Run starts the expander in a loop
func (e *Expander) Run(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := e.ExpandRecurringEvents(ctx); err != nil {
				// Log error but continue running
				fmt.Printf("Error expanding recurring events: %v\n", err)
			}
		}
	}
} 