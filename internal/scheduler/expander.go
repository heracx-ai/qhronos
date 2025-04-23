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
		if len(schedule.ByDay) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, 7*schedule.Interval) {
				if t.After(now) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			weekdayMap := map[string]time.Weekday{
				"SU": time.Sunday,
				"MO": time.Monday,
				"TU": time.Tuesday,
				"WE": time.Wednesday,
				"TH": time.Thursday,
				"FR": time.Friday,
				"SA": time.Saturday,
			}
			for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, 7*schedule.Interval) {
				for _, day := range schedule.ByDay {
					weekday := weekdayMap[day]
					daysToAdd := (int(weekday) - int(t.Weekday()) + 7) % 7
					nextDay := t.AddDate(0, 0, daysToAdd)
					if nextDay.After(now) && nextDay.Before(endTime) {
						occurrences = append(occurrences, nextDay)
					}
				}
			}
		}
	case "monthly":
		if len(schedule.ByMonthDay) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, schedule.Interval, 0) {
				if t.After(now) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, schedule.Interval, 0) {
				for _, day := range schedule.ByMonthDay {
					nextDay := time.Date(t.Year(), t.Month(), day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
					if nextDay.After(now) && nextDay.Before(endTime) {
						occurrences = append(occurrences, nextDay)
					}
				}
			}
		}
	case "yearly":
		if len(schedule.ByMonth) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(schedule.Interval, 0, 0) {
				if t.After(now) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			for t := startTime; t.Before(endTime); t = t.AddDate(schedule.Interval, 0, 0) {
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

	if schedule.Count != nil && len(occurrences) > *schedule.Count {
		occurrences = occurrences[:*schedule.Count]
	}

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

	for _, t := range occurrences {
		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  t,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    now,
		}
		e.scheduleWithRetry(ctx, occurrence)
	}

	return nil
}

func (e *Expander) expandNonRecurringEvent(ctx context.Context, event *models.Event) error {
	log.Printf("Expanding non-recurring event: id=%s", event.ID)

	if event.Schedule == nil {
		return fmt.Errorf("event schedule is nil")
	}

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

	occurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      event.ID,
		ScheduledAt:  startTime,
		Status:       models.OccurrenceStatusPending,
		Timestamp:    now,
	}
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
