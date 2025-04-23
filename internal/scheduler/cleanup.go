package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/feedloop/qhronos/internal/repository"
)

// CleanupService handles periodic deletion of old events and occurrences
type CleanupService struct {
	repo                 *repository.EventRepository
	eventsRetention     time.Duration
	occurrencesRetention time.Duration
	cleanupInterval     time.Duration
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(repo *repository.EventRepository, eventsRetention, occurrencesRetention, cleanupInterval time.Duration) *CleanupService {
	return &CleanupService{
		repo:                repo,
		eventsRetention:     eventsRetention,
		occurrencesRetention: occurrencesRetention,
		cleanupInterval:     cleanupInterval,
	}
}

// Run starts the cleanup service
func (c *CleanupService) Run(ctx context.Context) {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.cleanup(ctx); err != nil {
				log.Printf("Error during cleanup: %v", err)
			}
		}
	}
}

// cleanup performs the actual cleanup of old events and occurrences
func (c *CleanupService) cleanup(ctx context.Context) error {
	// Calculate cutoff times
	eventsCutoff := time.Now().Add(-c.eventsRetention)
	occurrencesCutoff := time.Now().Add(-c.occurrencesRetention)

	// Delete old occurrences first
	if err := c.repo.DeleteOldOccurrences(ctx, occurrencesCutoff); err != nil {
		return err
	}

	// Delete old events
	if err := c.repo.DeleteOldEvents(ctx, eventsCutoff); err != nil {
		return err
	}

	return nil
} 