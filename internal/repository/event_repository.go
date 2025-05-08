package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type EventRepository struct {
	db          *sqlx.DB
	logger      *zap.Logger
	redis       *redis.Client
	redisPrefix string
}

func NewEventRepository(db *sqlx.DB, logger *zap.Logger, redis *redis.Client, prefix string) *EventRepository {
	return &EventRepository{db: db, logger: logger, redis: redis, redisPrefix: prefix}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// deriveAndSetAction populates the event.Action field based on event.Webhook
// if Action is not already set. It also ensures Webhook is consistent if Action is set.
func (r *EventRepository) deriveAndSetAction(event *models.Event) error {
	// Always re-derive Action from Webhook if Webhook is set
	if event.Webhook != "" {
		var paramsBytes []byte
		var err error
		var actionType models.ActionType

		if len(event.Webhook) > 2 && event.Webhook[:2] == "q:" {
			actionType = models.ActionTypeWebsocket
			wsParams := models.WebsocketActionParams{ClientName: event.Webhook[2:]}
			paramsBytes, err = json.Marshal(wsParams)
		} else {
			actionType = models.ActionTypeWebhook
			whParams := models.WebhookActionParams{URL: event.Webhook}
			paramsBytes, err = json.Marshal(whParams)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal action params: %w", err)
		}
		event.Action = &models.Action{
			Type:   actionType,
			Params: paramsBytes,
		}
	} else if event.Action != nil && event.Webhook == "" {
		r.populateWebhookFromAction(event)
	}
	return nil
}

// populateWebhookFromAction sets the event.Webhook string based on event.Action.
// This is for backward compatibility for code that might still read the Webhook field.
func (r *EventRepository) populateWebhookFromAction(event *models.Event) {
	if event.Action == nil {
		return
	}
	switch event.Action.Type {
	case models.ActionTypeWebhook:
		params, err := event.Action.GetWebhookParams()
		if err != nil {
			r.logger.Error("Failed to get webhook params for populating Webhook string", zap.String("event_id", event.ID.String()), zap.Error(err))
			return
		}
		event.Webhook = params.URL
	case models.ActionTypeWebsocket:
		params, err := event.Action.GetWebsocketParams()
		if err != nil {
			r.logger.Error("Failed to get websocket params for populating Webhook string", zap.String("event_id", event.ID.String()), zap.Error(err))
			return
		}
		event.Webhook = "q:" + params.ClientName
	default:
		// If action type is unknown or params are malformed, Webhook might remain as it is or be empty.
		// Consider if event.Webhook should be cleared here if action type is unknown.
		r.logger.Warn("Unknown action type for populating Webhook string", zap.String("event_id", event.ID.String()), zap.String("action_type", string(event.Action.Type)))
	}
}

func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	query := `
		INSERT INTO events (id, name, description, schedule, start_time, metadata, webhook, tags, status, created_at, updated_at, action)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`

	now := time.Now()
	event.ID = uuid.New()
	event.CreatedAt = now
	if event.UpdatedAt == nil {
		event.UpdatedAt = timePtr(now)
	}
	if event.Status == "" {
		event.Status = models.EventStatusActive
	}

	if err := r.deriveAndSetAction(event); err != nil {
		return fmt.Errorf("error deriving action for event: %w", err)
	}

	err := r.db.QueryRowContext(ctx, query,
		event.ID,
		event.Name,
		event.Description,
		event.Schedule,
		event.StartTime,
		event.Metadata,
		event.Webhook, // Still pass webhook for now, migration will handle old data / or ensure deriveAndSetAction fills it if action exists
		event.Tags,
		event.Status,
		event.CreatedAt,
		event.UpdatedAt,
		event.Action,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("error creating event: %w", err)
	}

	return nil
}

func (r *EventRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook, tags, status, created_at, updated_at, action
		FROM events
		WHERE id = $1`

	var event models.Event
	err := r.db.GetContext(ctx, &event, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting event: %w", err)
	}
	r.populateWebhookFromAction(&event)
	return &event, nil
}

func (r *EventRepository) List(ctx context.Context) ([]models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook, tags, status, created_at, updated_at, action
		FROM events
		ORDER BY created_at DESC`

	var events []models.Event
	err := r.db.SelectContext(ctx, &events, query)
	if err != nil {
		return nil, fmt.Errorf("error listing events: %w", err)
	}

	for i := range events {
		r.populateWebhookFromAction(&events[i])
	}

	return events, nil
}

func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	query := `
		UPDATE events
		SET name = $1, description = $2, schedule = $3, start_time = $4, metadata = $5, webhook = $6, tags = $7, status = $8, updated_at = $9, action = $10
		WHERE id = $11
		RETURNING id`

	event.UpdatedAt = timePtr(time.Now())

	if err := r.deriveAndSetAction(event); err != nil {
		return fmt.Errorf("error deriving action for event update: %w", err)
	}

	result, err := r.db.ExecContext(ctx, query,
		event.Name,
		event.Description,
		event.Schedule,
		event.StartTime,
		event.Metadata,
		event.Webhook,
		event.Tags,
		event.Status,
		event.UpdatedAt,
		event.Action,
		event.ID,
	)

	if err != nil {
		return fmt.Errorf("error updating event: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return models.ErrEventNotFound
	}

	return nil
}

func (r *EventRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// First check if the event exists
	query := `SELECT id FROM events WHERE id = $1`
	var eventID uuid.UUID
	err := r.db.GetContext(ctx, &eventID, query, id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error checking event existence: %w", err)
	}

	// Update updated_at before deletion
	updateQuery := `UPDATE events SET updated_at = $1 WHERE id = $2`
	_, err = r.db.ExecContext(ctx, updateQuery, time.Now(), id)
	if err != nil {
		return fmt.Errorf("error updating event before deletion: %w", err)
	}

	// Delete the event
	deleteQuery := `DELETE FROM events WHERE id = $1`
	_, err = r.db.ExecContext(ctx, deleteQuery, id)
	if err != nil {
		return fmt.Errorf("error deleting event: %w", err)
	}

	// Remove all scheduled occurrences for this event from Redis
	err = r.RemoveEventOccurrencesFromRedis(ctx, id)
	if err != nil {
		r.logger.Warn("Failed to remove event occurrences from Redis", zap.String("event_id", id.String()), zap.Error(err))
	}

	return nil
}

// RemoveEventOccurrencesFromRedis removes all scheduled occurrences for an event from Redis
func (r *EventRepository) RemoveEventOccurrencesFromRedis(ctx context.Context, eventID uuid.UUID) error {
	results, err := r.redis.ZRange(ctx, r.redisPrefix+"schedules", 0, -1).Result()
	if err != nil {
		return err
	}
	for _, key := range results {
		// Fetch the schedule data from the hash
		data, err := r.redis.HGet(ctx, r.redisPrefix+"schedule:data", key).Result()
		if err != nil {
			continue // skip if not found or error
		}
		var sched struct {
			EventID uuid.UUID `json:"event_id"`
		}
		if err := json.Unmarshal([]byte(data), &sched); err != nil {
			continue // skip invalid
		}
		if sched.EventID == eventID {
			// Remove from both sorted set and hash
			r.redis.ZRem(ctx, r.redisPrefix+"schedules", key)
			r.redis.HDel(ctx, r.redisPrefix+"schedule:data", key)
		}
	}
	return nil
}

func (r *EventRepository) ListByTags(ctx context.Context, tags []string) ([]*models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook, tags, status, created_at, updated_at, action
		FROM events 
		WHERE tags && $1
		ORDER BY created_at DESC
	`

	var events []*models.Event
	err := r.db.SelectContext(ctx, &events, query, pq.Array(tags))
	if err != nil {
		return nil, fmt.Errorf("error listing events by tags: %w", err)
	}

	for i := range events {
		if events[i] != nil {
			r.populateWebhookFromAction(events[i])
		}
	}

	return events, nil
}

// DeleteOldEvents deletes events that are older than the specified cutoff time
func (r *EventRepository) DeleteOldEvents(ctx context.Context, cutoff time.Time) error {
	query := `
		DELETE FROM events 
		WHERE updated_at < $1
		AND id NOT IN (
			SELECT event_id 
			FROM occurrences 
			WHERE scheduled_at >= $1
			AND status != $2
		)
	`
	_, err := r.db.ExecContext(ctx, query, cutoff, models.OccurrenceStatusDispatched)
	return err
}

// DeleteOldOccurrences deletes occurrences that are older than the specified cutoff time
func (r *EventRepository) DeleteOldOccurrences(ctx context.Context, cutoff time.Time) error {
	query := `
		DELETE FROM occurrences 
		WHERE scheduled_at < $1
		AND status IN ($2, $3)
	`
	_, err := r.db.ExecContext(ctx, query, cutoff, models.OccurrenceStatusDispatched, models.OccurrenceStatusCompleted)
	return err
}

func (r *EventRepository) ListActive(ctx context.Context) ([]*models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook, tags, status, created_at, updated_at, action
		FROM events
		WHERE status = $1
		ORDER BY created_at DESC`

	var events []*models.Event
	err := r.db.SelectContext(ctx, &events, query, models.EventStatusActive)
	if err != nil {
		return nil, fmt.Errorf("error listing active events: %w", err)
	}

	for i := range events {
		if events[i] != nil {
			r.populateWebhookFromAction(events[i])
		}
	}

	return events, nil
}

func (r *EventRepository) CreateOccurrence(ctx context.Context, occurrence *models.Occurrence) error {
	query := `
		INSERT INTO occurrences (occurrence_id, event_id, scheduled_at, status, attempt_count, timestamp, status_code, response_body, error_message, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, timestamp`

	if occurrence.OccurrenceID == uuid.Nil {
		occurrence.OccurrenceID = uuid.New()
	}
	if occurrence.Timestamp.IsZero() {
		occurrence.Timestamp = time.Now()
	}

	err := r.db.QueryRowContext(ctx, query,
		occurrence.OccurrenceID,
		occurrence.EventID,
		occurrence.ScheduledAt,
		occurrence.Status,
		occurrence.AttemptCount,
		occurrence.Timestamp,
		occurrence.StatusCode,
		occurrence.ResponseBody,
		occurrence.ErrorMessage,
		occurrence.StartedAt,
		occurrence.CompletedAt,
	).Scan(&occurrence.ID, &occurrence.Timestamp)

	if err != nil {
		return fmt.Errorf("error creating occurrence: %w", err)
	}

	return nil
}

func (r *EventRepository) GetOccurrenceByID(ctx context.Context, id int) (*models.Occurrence, error) {
	query := `
		SELECT id, occurrence_id, event_id, scheduled_at, status, attempt_count, timestamp, status_code, response_body, error_message, started_at, completed_at
		FROM occurrences
		WHERE id = $1`

	var occurrence models.Occurrence
	err := r.db.GetContext(ctx, &occurrence, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting occurrence: %w", err)
	}

	return &occurrence, nil
}

func (r *EventRepository) CreateEvent(ctx context.Context, event *models.Event) error {
	query := `
		INSERT INTO events (
			id, name, description, start_time, webhook, 
			metadata, schedule, tags, status, hmac_secret
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	var scheduleJSON []byte
	if event.Schedule != nil {
		var err error
		scheduleJSON, err = json.Marshal(event.Schedule)
		if err != nil {
			return fmt.Errorf("failed to marshal schedule: %w", err)
		}
	}

	_, err := r.db.ExecContext(ctx, query,
		event.ID,
		event.Name,
		event.Description,
		event.StartTime,
		event.Webhook,
		event.Metadata,
		scheduleJSON,
		pq.Array(event.Tags),
		event.Status,
		event.HMACSecret,
	)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return nil
}

func (r *EventRepository) GetEvent(ctx context.Context, id string) (*models.Event, error) {
	eventID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid event ID format: %w", err)
	}
	return r.GetByID(ctx, eventID)
}

func (r *EventRepository) ListEvents(ctx context.Context, filter models.EventFilter) ([]*models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook, tags, status, created_at, updated_at, action
		FROM events
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, filter.Status)
		argCount++
	}

	if len(filter.Tags) > 0 {
		query += fmt.Sprintf(" AND tags && $%d", argCount)
		args = append(args, pq.Array(filter.Tags))
		argCount++
	}

	if filter.StartTimeBefore != nil {
		query += fmt.Sprintf(" AND start_time <= $%d", argCount)
		args = append(args, filter.StartTimeBefore)
		argCount++
	}

	if filter.StartTimeAfter != nil {
		query += fmt.Sprintf(" AND start_time >= $%d", argCount)
		args = append(args, filter.StartTimeAfter)
		argCount++
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*models.Event
	for rows.Next() {
		var event models.Event
		var scheduleJSON []byte
		err := rows.Scan(
			&event.ID,
			&event.Name,
			&event.Description,
			&event.StartTime,
			&event.Webhook,
			&event.Metadata,
			&scheduleJSON,
			pq.Array(&event.Tags),
			&event.Status,
			&event.CreatedAt,
			&event.UpdatedAt,
			&event.Action,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if len(scheduleJSON) > 0 {
			var schedule models.ScheduleConfig
			if err := json.Unmarshal(scheduleJSON, &schedule); err != nil {
				return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
			}
			event.Schedule = &schedule
		}

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	for i := range events {
		if events[i] != nil {
			r.populateWebhookFromAction(events[i])
		}
	}

	return events, nil
}
