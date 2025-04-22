package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type EventRepository struct {
	db *sqlx.DB
}

func NewEventRepository(db *sqlx.DB) *EventRepository {
	return &EventRepository{db: db}
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
	event.UpdatedAt = timePtr(now)
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