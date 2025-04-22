package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/feedloop/qhronos/internal/models"
	"github.com/lib/pq"
)

type OccurrenceRepository struct {
	db *sqlx.DB
}

func NewOccurrenceRepository(db *sqlx.DB) *OccurrenceRepository {
	return &OccurrenceRepository{db: db}
}

func (r *OccurrenceRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Occurrence, error) {
	var occurrence models.Occurrence
	query := `SELECT * FROM occurrences WHERE id = $1`
	err := r.db.GetContext(ctx, &occurrence, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &occurrence, err
}

func (r *OccurrenceRepository) ListByTags(ctx context.Context, filter models.OccurrenceFilter) ([]models.Occurrence, int, error) {
	var total int

	// Count total records
	countQuery := `
		SELECT COUNT(DISTINCT o.id) FROM occurrences o
		JOIN events e ON o.event_id = e.id
		WHERE e.tags @> $1 AND e.status != 'deleted'`
	
	err := r.db.GetContext(ctx, &total, countQuery, pq.Array(filter.Tags))
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := `
		SELECT DISTINCT o.* FROM occurrences o
		JOIN events e ON o.event_id = e.id
		WHERE e.tags @> $1 AND e.status != 'deleted'
		ORDER BY o.scheduled_at DESC
		LIMIT $2 OFFSET $3`

	offset := (filter.Page - 1) * filter.Limit
	
	// Use a temporary struct to handle the array type
	var tempOccurrences []struct {
		ID            uuid.UUID   `db:"id"`
		EventID       uuid.UUID   `db:"event_id"`
		ScheduledAt   time.Time   `db:"scheduled_at"`
		Status        string      `db:"status"`
		LastAttempt   *time.Time  `db:"last_attempt"`
		AttemptCount  int         `db:"attempt_count"`
		CreatedAt     time.Time   `db:"created_at"`
	}

	err = r.db.SelectContext(ctx, &tempOccurrences, query, pq.Array(filter.Tags), filter.Limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Convert to the actual Occurrence struct
	occurrences := make([]models.Occurrence, len(tempOccurrences))
	for i, tempOccurrence := range tempOccurrences {
		occurrences[i] = models.Occurrence{
			ID:           tempOccurrence.ID,
			EventID:      tempOccurrence.EventID,
			ScheduledAt:  tempOccurrence.ScheduledAt,
			Status:       models.OccurrenceStatus(tempOccurrence.Status),
			LastAttempt:  tempOccurrence.LastAttempt,
			AttemptCount: tempOccurrence.AttemptCount,
			CreatedAt:    tempOccurrence.CreatedAt,
		}
	}

	return occurrences, total, nil
}

func (r *OccurrenceRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.OccurrenceStatus) error {
	query := `
		UPDATE occurrences SET
			status = $1,
			last_attempt = $2,
			attempt_count = attempt_count + 1
		WHERE id = $3`

	_, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	return err
}

func (r *OccurrenceRepository) Create(ctx context.Context, occurrence *models.Occurrence) error {
	query := `
		INSERT INTO occurrences (id, event_id, scheduled_at, status, last_attempt, attempt_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.db.ExecContext(ctx, query,
		occurrence.ID,
		occurrence.EventID,
		occurrence.ScheduledAt,
		occurrence.Status,
		occurrence.LastAttempt,
		occurrence.AttemptCount,
		occurrence.CreatedAt,
	)
	return err
} 