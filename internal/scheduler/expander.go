package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Expander struct {
	scheduler         *Scheduler
	eventRepo         *repository.EventRepository
	occurrenceRepo    *repository.OccurrenceRepository
	lookAheadDuration time.Duration
	expansionInterval time.Duration
	gracePeriod       time.Duration
	logger            *zap.Logger
}

// NewExpander creates a new Expander with configurable look-ahead duration and expansion interval
func NewExpander(
	scheduler *Scheduler,
	eventRepo *repository.EventRepository,
	occurrenceRepo *repository.OccurrenceRepository,
	lookAheadDuration time.Duration,
	expansionInterval time.Duration,
	gracePeriod time.Duration,
	logger *zap.Logger,
) *Expander {
	logger.Debug("Initializing Expander", zap.Duration("lookAheadDuration", lookAheadDuration), zap.Duration("expansionInterval", expansionInterval))
	return &Expander{
		scheduler:         scheduler,
		eventRepo:         eventRepo,
		occurrenceRepo:    occurrenceRepo,
		lookAheadDuration: lookAheadDuration,
		expansionInterval: expansionInterval,
		gracePeriod:       gracePeriod,
		logger:            logger,
	}
}

// ExpandEvents expands both recurring and non-recurring events into occurrences
func (e *Expander) ExpandEvents(ctx context.Context) error {
	e.logger.Debug("Starting event expansion", zap.Duration("lookAheadDuration", e.lookAheadDuration))

	// Get active events
	events, err := e.eventRepo.ListActive(ctx)
	if err != nil {
		e.logger.Error("Error getting active events", zap.Error(err))
		return fmt.Errorf("failed to get active events: %w", err)
	}
	e.logger.Debug("Found active events to expand", zap.Int("count", len(events)))

	for _, event := range events {
		e.logger.Debug("Processing event", zap.String("event_id", event.ID.String()), zap.String("name", event.Name))

		// Handle recurring events
		if event.Schedule != nil {
			if err := e.expandRecurringEvent(ctx, event); err != nil {
				e.logger.Error("Error expanding recurring event", zap.String("event_id", event.ID.String()), zap.Error(err))
				continue
			}
		} else {
			// Handle non-recurring events
			if err := e.expandNonRecurringEvent(ctx, event); err != nil {
				e.logger.Error("Error expanding non-recurring event", zap.String("event_id", event.ID.String()), zap.Error(err))
				continue
			}
		}
	}
	e.logger.Debug("Completed event expansion")
	return nil
}

// scheduleWithRetry tries to schedule a schedule object in Redis, retrying on timeout/connection errors
func (e *Expander) scheduleWithRetry(ctx context.Context, occurrence *models.Occurrence, event *models.Event) {
	const maxRetries = 3
	delays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := e.scheduler.ScheduleEvent(ctx, occurrence, event)
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
			e.logger.Error("Failed to schedule occurrence in Redis (non-retryable)", zap.Error(err))
			return
		}
	}
	if lastErr != nil {
		e.logger.Error("Failed to schedule occurrence in Redis after retries", zap.Error(lastErr))
	}
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
	e.logger.Debug("Expanding recurring event", zap.String("event_id", event.ID.String()))

	// Get next occurrences based on schedule configuration
	now := time.Now().UTC()
	graceStart := now.Add(-e.gracePeriod)
	endTime := now.Add(e.lookAheadDuration)
	startTime := event.StartTime.UTC()

	var occurrences []time.Time
	schedule := event.Schedule

	// Calculate occurrences based on frequency
	switch schedule.Frequency {
	case "daily":
		for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, schedule.Interval) {
			if t.After(graceStart) {
				occurrences = append(occurrences, t)
			}
		}
	case "weekly":
		if len(schedule.ByDay) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, 0, 7*schedule.Interval) {
				if t.After(graceStart) {
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
					if nextDay.After(graceStart) && nextDay.Before(endTime) {
						occurrences = append(occurrences, nextDay)
					}
				}
			}
		}
	case "monthly":
		if len(schedule.ByMonthDay) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, schedule.Interval, 0) {
				if t.After(graceStart) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			for t := startTime; t.Before(endTime); t = t.AddDate(0, schedule.Interval, 0) {
				for _, day := range schedule.ByMonthDay {
					nextDay := time.Date(t.Year(), t.Month(), day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
					if nextDay.After(graceStart) && nextDay.Before(endTime) {
						occurrences = append(occurrences, nextDay)
					}
				}
			}
		}
	case "yearly":
		if len(schedule.ByMonth) == 0 {
			for t := startTime; t.Before(endTime); t = t.AddDate(schedule.Interval, 0, 0) {
				if t.After(graceStart) {
					occurrences = append(occurrences, t)
				}
			}
		} else {
			for t := startTime; t.Before(endTime); t = t.AddDate(schedule.Interval, 0, 0) {
				for _, month := range schedule.ByMonth {
					nextMonth := time.Date(t.Year(), time.Month(month), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
					if nextMonth.After(graceStart) && nextMonth.Before(endTime) {
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

	e.logger.Debug("Found future occurrences for event", zap.String("event_id", event.ID.String()), zap.Int("count", len(occurrences)))

	for _, t := range occurrences {
		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  t,
			Status:       models.OccurrenceStatusPending,
			Timestamp:    now,
		}
		e.scheduleWithRetry(ctx, occurrence, event)
	}

	return nil
}

func (e *Expander) expandNonRecurringEvent(ctx context.Context, event *models.Event) error {
	e.logger.Debug("Expanding non-recurring event", zap.String("event_id", event.ID.String()))

	// For non-recurring events, Schedule should be nil. If not, log a warning but proceed.
	if event.Schedule != nil {
		e.logger.Warn("Non-recurring event has a non-nil schedule; ignoring schedule", zap.String("event_id", event.ID.String()))
	}

	now := time.Now().UTC()
	graceStart := now.Add(-e.gracePeriod)
	startTime := event.StartTime.UTC()

	endTime := now.Add(e.lookAheadDuration)
	if startTime.After(endTime) {
		e.logger.Info("Event is beyond look-ahead window, skipping", zap.String("event_id", event.ID.String()))
		return nil
	}
	if startTime.Before(graceStart) {
		e.logger.Info("Event is before grace period, skipping", zap.String("event_id", event.ID.String()))
		return nil
	}

	occurrence := &models.Occurrence{
		OccurrenceID: uuid.New(),
		EventID:      event.ID,
		ScheduledAt:  startTime,
		Status:       models.OccurrenceStatusPending,
		Timestamp:    now,
	}
	e.scheduleWithRetry(ctx, occurrence, event)

	return nil
}

// Run starts the expander in a loop
func (e *Expander) Run(ctx context.Context) error {
	e.logger.Info("Starting Expander service", zap.Duration("interval", e.expansionInterval))
	ticker := time.NewTicker(e.expansionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("Expander service shutting down")
			return ctx.Err()
		case <-ticker.C:
			if err := e.ExpandEvents(ctx); err != nil {
				e.logger.Error("Error during event expansion", zap.Error(err))
			}
		}
	}
}

func (e *Expander) ExpandRecurringEvent(ctx context.Context, event *models.Event) error {
	return e.expandRecurringEvent(ctx, event)
}

func (e *Expander) ExpandNonRecurringEvent(ctx context.Context, event *models.Event) error {
	return e.expandNonRecurringEvent(ctx, event)
}

func (e *Expander) GracePeriod() time.Duration {
	return e.gracePeriod
}

func (e *Expander) LookAheadDuration() time.Duration {
	return e.lookAheadDuration
}
