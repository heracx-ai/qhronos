package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/google/uuid"
)

type Expander struct {
	scheduler         *Scheduler
	eventRepo         *repository.EventRepository
	occurrenceRepo    *repository.OccurrenceRepository
	lookAheadDuration time.Duration
	expansionInterval time.Duration
}

// NewExpander creates a new Expander with configurable look-ahead duration and expansion interval
func NewExpander(
	scheduler *Scheduler,
	eventRepo *repository.EventRepository,
	occurrenceRepo *repository.OccurrenceRepository,
	lookAheadDuration time.Duration,
	expansionInterval time.Duration,
) *Expander {
	log.Printf("Initializing Expander with lookAheadDuration=%v, expansionInterval=%v",
		lookAheadDuration, expansionInterval)
	return &Expander{
		scheduler:         scheduler,
		eventRepo:         eventRepo,
		occurrenceRepo:    occurrenceRepo,
		lookAheadDuration: lookAheadDuration,
		expansionInterval: expansionInterval,
	}
}

// ExpandEvents expands both recurring and non-recurring events into occurrences
func (e *Expander) ExpandEvents(ctx context.Context) error {
	log.Printf("Starting event expansion with look-ahead duration: %v", e.lookAheadDuration)

	// Get active events
	events, err := e.eventRepo.ListActive(ctx)
	if err != nil {
		log.Printf("Error getting active events: %v", err)
		return fmt.Errorf("failed to get active events: %w", err)
	}
	log.Printf("Found %d active events to expand", len(events))

	for _, event := range events {
		log.Printf("Processing event: id=%s, name=%s", event.ID, event.Name)

		// Handle recurring events
		if event.Schedule != nil {
			if err := e.expandRecurringEvent(ctx, event); err != nil {
				log.Printf("Error expanding recurring event %s: %v", event.ID, err)
				continue
			}
		} else {
			// Handle non-recurring events
			if err := e.expandNonRecurringEvent(ctx, event); err != nil {
				log.Printf("Error expanding non-recurring event %s: %v", event.ID, err)
				continue
			}
		}
	}

	log.Printf("Completed event expansion")
	return nil
}

// scheduleWithRetry tries to schedule an occurrence in Redis, retrying on timeout/connection errors
func (e *Expander) scheduleWithRetry(ctx context.Context, occurrence *models.Occurrence) {
	const maxRetries = 3
	delays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := e.scheduler.ScheduleEvent(ctx, occurrence)
		if err == nil {
			return
		}
		// Check for timeout/connection errors (by error string, since go-redis/v9 does not export error types)
		errStr := err.Error()
		if isRedisTimeoutOrConnError(errStr) {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(delays[i])
				continue
			}
		} else {
			log.Printf("Failed to schedule occurrence in Redis (non-retryable): %v", err)
			return
		}
	}
	log.Printf("Failed to schedule occurrence in Redis after retries: %v", lastErr)
}

// isRedisTimeoutOrConnError checks if the error string indicates a timeout or connection error
func isRedisTimeoutOrConnError(errStr string) bool {
	return (contains(errStr, "timeout") || contains(errStr, "connection refused") || contains(errStr, "i/o timeout") || contains(errStr, "network is unreachable"))
}

// contains is a helper for substring search
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr))))
}

func (e *Expander) expandRecurringEvent(ctx context.Context, event *models.Event) error {
	log.Printf("Expanding recurring event: id=%s", event.ID)

	// Get next occurrences based on schedule configuration
	now := time.Now().UTC()
	endTime := now.Add(e.lookAheadDuration)
	startTime := event.StartTime.UTC()

	var occurrences []time.Time
	schedule := event.Schedule

	// Calculate occurrences based on frequency
	switch schedule.Frequency {
	case "daily":
		for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, schedule.Interval) {
			if t.After(now) {
				occurrences = append(occurrences, t)
			}
		}
	case "weekly":
		// If no specific days are specified, use the start time's day
		if len(schedule.ByDay) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, 7*schedule.Interval) {
				if t.After(now) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			// Map day strings to time.Weekday
			weekdayMap := map[string]time.Weekday{
				"SU": time.Sunday,
				"MO": time.Monday,
				"TU": time.Tuesday,
				"WE": time.Wednesday,
				"TH": time.Thursday,
				"FR": time.Friday,
				"SA": time.Saturday,
			}

			// For each week in the interval
			for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, 7*schedule.Interval) {
				// For each specified day in the week
				for _, day := range schedule.ByDay {
					weekday := weekdayMap[day]
					// Calculate the next occurrence of this weekday
					daysToAdd := (int(weekday) - int(t.Weekday()) + 7) % 7
					nextDay := t.AddDate(0, 0, daysToAdd)
					if nextDay.After(now) && nextDay.Before(endTime) {
						occurrences = append(occurrences, nextDay)
					}
				}
			}
		}
	case "monthly":
		// If no specific days are specified, use the start time's day
		if len(schedule.ByMonthDay) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, schedule.Interval, 0) {
				if t.After(now) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			// For each month in the interval
			for t := startTime; t.Before(endTime); t = t.AddDate(0, schedule.Interval, 0) {
				// For each specified day in the month
				for _, day := range schedule.ByMonthDay {
					nextDay := time.Date(t.Year(), t.Month(), day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
					if nextDay.After(now) && nextDay.Before(endTime) {
						occurrences = append(occurrences, nextDay)
					}
				}
			}
		}
	case "yearly":
		// If no specific months are specified, use the start time's month
		if len(schedule.ByMonth) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(schedule.Interval, 0, 0) {
				if t.After(now) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			// For each year in the interval
			for t := startTime; t.Before(endTime); t = t.AddDate(schedule.Interval, 0, 0) {
				// For each specified month in the year
				for _, month := range schedule.ByMonth {
					nextMonth := time.Date(t.Year(), time.Month(month), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
					if nextMonth.After(now) && nextMonth.Before(endTime) {
						occurrences = append(occurrences, nextMonth)
					}
				}
			}
		}
	default:
		return fmt.Errorf("invalid frequency: %s", schedule.Frequency)
	}

	// Apply count limit if specified
	if schedule.Count != nil && len(occurrences) > *schedule.Count {
		occurrences = occurrences[:*schedule.Count]
	}

	// Apply until limit if specified
	if schedule.Until != nil {
		untilTime, err := time.Parse(time.RFC3339, *schedule.Until)
		if err != nil {
			return fmt.Errorf("invalid until date: %w", err)
		}
		var filteredOccurrences []time.Time
		for _, t := range occurrences {
			if t.Before(untilTime) {
				filteredOccurrences = append(filteredOccurrences, t)
			}
		}
		occurrences = filteredOccurrences
	}

	log.Printf("Found %d future occurrences for event %s", len(occurrences), event.ID)

	// Create occurrences
	for _, t := range occurrences {
		exists, err := e.occurrenceRepo.ExistsAtTime(ctx, event.ID, t)
		if err != nil {
			log.Printf("Error checking occurrence existence for event %s at %v: %v", event.ID, t, err)
			continue
		}
		if exists {
			log.Printf("Occurrence already exists for event %s at %v", event.ID, t)
			continue
		}

		occurrence := &models.Occurrence{
			ID:          uuid.New(),
			EventID:     event.ID,
			ScheduledAt: t,
			Status:      models.OccurrenceStatusPending,
			CreatedAt:   now,
		}

		if err := e.occurrenceRepo.Create(ctx, occurrence); err != nil {
			log.Printf("Error creating occurrence for event %s at %v: %v", event.ID, t, err)
			continue
		}
		log.Printf("Created new occurrence: id=%s, event_id=%s, scheduled_at=%v",
			occurrence.ID, event.ID, t)
		// Schedule in Redis with retry
		e.scheduleWithRetry(ctx, occurrence)
	}

	return nil
}

func (e *Expander) expandNonRecurringEvent(ctx context.Context, event *models.Event) error {
	log.Printf("Expanding non-recurring event: id=%s", event.ID)

	if event.Schedule == nil {
		return fmt.Errorf("event schedule is nil")
	}

	// Check if the event is in the future and within look-ahead window
	now := time.Now().UTC()
	startTime := event.StartTime.UTC()

	if startTime.Before(now) {
		log.Printf("Event %s is in the past, skipping", event.ID)
		return nil
	}

	endTime := now.Add(e.lookAheadDuration)
	if startTime.After(endTime) {
		log.Printf("Event %s is beyond look-ahead window, skipping", event.ID)
		return nil
	}

	// Check if occurrence already exists
	exists, err := e.occurrenceRepo.ExistsAtTime(ctx, event.ID, startTime)
	if err != nil {
		log.Printf("Error checking occurrence existence for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to check occurrence existence: %w", err)
	}
	if exists {
		log.Printf("Occurrence already exists for event %s", event.ID)
		return nil
	}

	// Create occurrence
	occurrence := &models.Occurrence{
		ID:          uuid.New(),
		EventID:     event.ID,
		ScheduledAt: startTime,
		Status:      models.OccurrenceStatusPending,
		CreatedAt:   now,
	}

	if err := e.occurrenceRepo.Create(ctx, occurrence); err != nil {
		log.Printf("Error creating occurrence for event %s: %v", event.ID, err)
		return fmt.Errorf("failed to create occurrence: %w", err)
	}
	log.Printf("Created new occurrence for non-recurring event: id=%s, event_id=%s, scheduled_at=%v",
		occurrence.ID, event.ID, startTime)
	// Schedule in Redis with retry
	e.scheduleWithRetry(ctx, occurrence)

	return nil
}

// Run starts the expander in a loop
func (e *Expander) Run(ctx context.Context) error {
	log.Printf("Starting Expander service with interval %v", e.expansionInterval)
	ticker := time.NewTicker(e.expansionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Expander service shutting down")
			return ctx.Err()
		case <-ticker.C:
			if err := e.ExpandEvents(ctx); err != nil {
				log.Printf("Error during event expansion: %v", err)
			}
		}
	}
}
