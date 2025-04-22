package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/feedloop/qhronos/internal/models"
)

type EventRepository struct {
	db *sqlx.DB
}

func NewEventRepository(db *sqlx.DB) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	query := `
		INSERT INTO events (
			id, title, start_time, webhook_url, payload, recurrence, tags, status, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)`

	_, err := r.db.ExecContext(ctx, query,
		event.ID,
		event.Title,
		event.StartTime,
		event.WebhookURL,
		event.Payload,
		event.Recurrence,
		pq.Array(event.Tags),
		event.Status,
		event.CreatedAt,
	)
	return err
}

func (r *EventRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Event, error) {
	query := `SELECT id, title, start_time, webhook_url, payload, recurrence, tags, status, created_at, updated_at FROM events WHERE id = $1 AND status != 'deleted'`
	
	// Use a temporary struct to handle the array type
	var tempEvent struct {
		ID         uuid.UUID   `db:"id"`
		Title      string      `db:"title"`
		StartTime  time.Time   `db:"start_time"`
		WebhookURL string      `db:"webhook_url"`
		Payload    []byte      `db:"payload"`
		Recurrence *string     `db:"recurrence"`
		Tags       pq.StringArray `db:"tags"`
		Status     string      `db:"status"`
		CreatedAt  time.Time   `db:"created_at"`
		UpdatedAt  *time.Time  `db:"updated_at"`
	}
	
	err := r.db.GetContext(ctx, &tempEvent, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Convert to the actual Event struct
	event := &models.Event{
		ID:         tempEvent.ID,
		Title:      tempEvent.Title,
		StartTime:  tempEvent.StartTime,
		WebhookURL: tempEvent.WebhookURL,
		Payload:    tempEvent.Payload,
		Recurrence: tempEvent.Recurrence,
		Tags:       []string(tempEvent.Tags),
		Status:     models.EventStatus(tempEvent.Status),
		CreatedAt:  tempEvent.CreatedAt,
		UpdatedAt:  tempEvent.UpdatedAt,
	}

	return event, nil
}

func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	query := `
		UPDATE events SET
			title = $1,
			start_time = $2,
			webhook_url = $3,
			payload = $4,
			recurrence = $5,
			tags = $6,
			status = $7,
			updated_at = $8
		WHERE id = $9 AND status != 'deleted'`

	result, err := r.db.ExecContext(ctx, query,
		event.Title,
		event.StartTime,
		event.WebhookURL,
		event.Payload,
		event.Recurrence,
		pq.Array(event.Tags),
		event.Status,
		event.UpdatedAt,
		event.ID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *EventRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// First check if the event exists and is not already deleted
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM events WHERE id = $1 AND status != 'deleted')", id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}

	// Start a transaction to handle both event and occurrences
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()

	// Delete associated occurrences first
	_, err = tx.ExecContext(ctx, `DELETE FROM occurrences WHERE event_id = $1`, id)
	if err != nil {
		return err
	}

	// Perform the soft delete on the event
	_, err = tx.ExecContext(ctx, `UPDATE events SET status = 'deleted', updated_at = $1 WHERE id = $2 AND status != 'deleted'`, now, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *EventRepository) ListByTags(ctx context.Context, tags []string) ([]models.Event, error) {
	query := `
		SELECT id, title, start_time, webhook_url, payload, recurrence, tags, status, created_at, updated_at
		FROM events
		WHERE tags @> $1 AND status != 'deleted'
		ORDER BY created_at DESC
	`

	// Use a temporary struct to handle the array type
	var tempEvents []struct {
		ID         uuid.UUID   `db:"id"`
		Title      string      `db:"title"`
		StartTime  time.Time   `db:"start_time"`
		WebhookURL string      `db:"webhook_url"`
		Payload    []byte      `db:"payload"`
		Recurrence *string     `db:"recurrence"`
		Tags       pq.StringArray `db:"tags"`
		Status     string      `db:"status"`
		CreatedAt  time.Time   `db:"created_at"`
		UpdatedAt  *time.Time  `db:"updated_at"`
	}

	err := r.db.SelectContext(ctx, &tempEvents, query, pq.Array(tags))
	if err != nil {
		return nil, err
	}

	// Convert to the actual Event struct
	events := make([]models.Event, len(tempEvents))
	for i, tempEvent := range tempEvents {
		events[i] = models.Event{
			ID:         tempEvent.ID,
			Title:      tempEvent.Title,
			StartTime:  tempEvent.StartTime,
			WebhookURL: tempEvent.WebhookURL,
			Payload:    tempEvent.Payload,
			Recurrence: tempEvent.Recurrence,
			Tags:       []string(tempEvent.Tags),
			Status:     models.EventStatus(tempEvent.Status),
			CreatedAt:  tempEvent.CreatedAt,
			UpdatedAt:  tempEvent.UpdatedAt,
		}
	}

	return events, nil
} 