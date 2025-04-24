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
	db     *sqlx.DB
	logger *zap.Logger
	redis  *redis.Client
}

func NewEventRepository(db *sqlx.DB, logger *zap.Logger, redis *redis.Client) *EventRepository {
	return &EventRepository{db: db, logger: logger, redis: redis}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	query := `
		INSERT INTO events (id, name, description, schedule, start_time, metadata, webhook_url, tags, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
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

	err := r.db.QueryRowContext(ctx, query,
		event.ID,
		event.Name,
		event.Description,
		event.Schedule,
		event.StartTime,
		event.Metadata,
		event.WebhookURL,
		event.Tags,
		event.Status,
		event.CreatedAt,
		event.UpdatedAt,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("error creating event: %w", err)
	}

	return nil
}

func (r *EventRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook_url, tags, status, created_at, updated_at
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

	return &event, nil
}

func (r *EventRepository) List(ctx context.Context) ([]models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook_url, tags, status, created_at, updated_at
		FROM events
		ORDER BY created_at DESC`

	var events []models.Event
	err := r.db.SelectContext(ctx, &events, query)
	if err != nil {
		return nil, fmt.Errorf("error listing events: %w", err)
	}

	return events, nil
}

func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	query := `
		UPDATE events
		SET name = $1, description = $2, schedule = $3, start_time = $4, metadata = $5, webhook_url = $6, tags = $7, status = $8, updated_at = $9
		WHERE id = $10
		RETURNING id`

	event.UpdatedAt = timePtr(time.Now())

	result, err := r.db.ExecContext(ctx, query,
		event.Name,
		event.Description,
		event.Schedule,
		event.StartTime,
		event.Metadata,
		event.WebhookURL,
		event.Tags,
		event.Status,
		event.UpdatedAt,
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
	err = r.removeEventOccurrencesFromRedis(ctx, id)
	if err != nil {
		r.logger.Warn("Failed to remove event occurrences from Redis", zap.String("event_id", id.String()), zap.Error(err))
	}

	// Remove recurring event from Redis (if present)
	_ = r.redis.HDel(ctx, "recurring:events", id.String()).Err()

	return nil
}

// removeEventOccurrencesFromRedis removes all scheduled occurrences for an event from Redis
func (r *EventRepository) removeEventOccurrencesFromRedis(ctx context.Context, eventID uuid.UUID) error {
	results, err := r.redis.ZRange(ctx, "schedule:events", 0, -1).Result()
	if err != nil {
		return err
	}
	for _, res := range results {
		var occ models.Occurrence
		if err := json.Unmarshal([]byte(res), &occ); err != nil {
			continue // skip invalid
		}
		if occ.EventID == eventID {
			r.redis.ZRem(ctx, "schedule:events", res)
		}
	}
	return nil
}

func (r *EventRepository) ListByTags(ctx context.Context, tags []string) ([]*models.Event, error) {
	query := `
		SELECT id, name, description, schedule, start_time, metadata, webhook_url, tags, status, created_at, updated_at
		FROM events 
		WHERE tags && $1
		ORDER BY created_at DESC
	`

	var events []*models.Event
	err := r.db.SelectContext(ctx, &events, query, pq.Array(tags))
	if err != nil {
		return nil, fmt.Errorf("error listing events by tags: %w", err)
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
		SELECT id, name, description, schedule, start_time, metadata, webhook_url, tags, status, created_at, updated_at
		FROM events
		WHERE status = $1
		ORDER BY created_at DESC`

	var events []*models.Event
	err := r.db.SelectContext(ctx, &events, query, models.EventStatusActive)
	if err != nil {
		return nil, fmt.Errorf("error listing active events: %w", err)
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
			id, name, description, start_time, webhook_url, 
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
		event.WebhookURL,
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
	query := `
		SELECT id, name, description, start_time, webhook_url, 
			metadata, schedule, tags, status, hmac_secret, created_at, updated_at
		FROM events
		WHERE id = $1
	`

	var event models.Event
	var scheduleJSON []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&event.ID,
		&event.Name,
		&event.Description,
		&event.StartTime,
		&event.WebhookURL,
		&event.Metadata,
		&scheduleJSON,
		pq.Array(&event.Tags),
		&event.Status,
		&event.HMACSecret,
		&event.CreatedAt,
		&event.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("event not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	if len(scheduleJSON) > 0 {
		var schedule models.ScheduleConfig
		if err := json.Unmarshal(scheduleJSON, &schedule); err != nil {
			return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
		}
		event.Schedule = &schedule
	}

	return &event, nil
}

func (r *EventRepository) ListEvents(ctx context.Context, filter models.EventFilter) ([]*models.Event, error) {
	query := `
		SELECT id, name, description, start_time, webhook_url, 
			metadata, schedule, tags, status, hmac_secret, created_at, updated_at
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
			&event.WebhookURL,
			&event.Metadata,
			&scheduleJSON,
			pq.Array(&event.Tags),
			&event.Status,
			&event.HMACSecret,
			&event.CreatedAt,
			&event.UpdatedAt,
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

	return events, nil
}
