package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type OccurrenceRepository struct {
	db *sqlx.DB
}

func NewOccurrenceRepository(db *sqlx.DB) *OccurrenceRepository {
	log.Printf("Initializing OccurrenceRepository")
	return &OccurrenceRepository{db: db}
}

func (r *OccurrenceRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Occurrence, error) {
	log.Printf("Getting occurrence by ID: %s", id)
	var occurrence models.Occurrence
	query := `SELECT * FROM occurrences WHERE id = $1`
	err := r.db.GetContext(ctx, &occurrence, query, id)
	if err == sql.ErrNoRows {
		log.Printf("Occurrence not found: %s", id)
		return nil, nil
	}
	if err != nil {
		log.Printf("Error getting occurrence %s: %v", id, err)
		return nil, err
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
		ID           uuid.UUID  `db:"id"`
		EventID      uuid.UUID  `db:"event_id"`
		ScheduledAt  time.Time  `db:"scheduled_at"`
		Status       string     `db:"status"`
		LastAttempt  *time.Time `db:"last_attempt"`
		AttemptCount int        `db:"attempt_count"`
		CreatedAt    time.Time  `db:"created_at"`
		UpdatedAt    *time.Time `db:"updated_at"`
	}

	err = r.db.SelectContext(ctx, &tempOccurrences, query, pq.Array(filter.Tags), filter.Limit, offset)
	if err != nil {
		log.Printf("ListByTags SQL error: %v", err)
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
			UpdatedAt:    tempOccurrence.UpdatedAt,
		}
	}

	return occurrences, total, nil
}

func (r *OccurrenceRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.OccurrenceStatus) error {
	log.Printf("Updating occurrence status: id=%s, status=%s", id, status)
	// First check if the occurrence exists
	var exists bool
	err := r.db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM occurrences WHERE id = $1)", id)
	if err != nil {
		log.Printf("Error checking occurrence existence: %v", err)
		return fmt.Errorf("error checking occurrence existence: %w", err)
	}
	if !exists {
		log.Printf("Occurrence not found for status update: %s", id)
		return models.ErrOccurrenceNotFound
	}

	query := `
		UPDATE occurrences SET
			status = $1,
			last_attempt = $2,
			attempt_count = attempt_count + 1
		WHERE id = $3`

	result, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		log.Printf("Error updating occurrence status: %v", err)
		return err
	}
	rows, _ := result.RowsAffected()
	log.Printf("Updated occurrence status: id=%s, status=%s, rows=%d", id, status, rows)
	return nil
}

func (r *OccurrenceRepository) Create(ctx context.Context, occurrence *models.Occurrence) error {
	log.Printf("Creating occurrence: id=%s, event_id=%s, scheduled_at=%v",
		occurrence.ID, occurrence.EventID, occurrence.ScheduledAt)
	query := `
		INSERT INTO occurrences (id, event_id, scheduled_at, status, last_attempt, attempt_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	result, err := r.db.ExecContext(ctx, query,
		occurrence.ID,
		occurrence.EventID,
		occurrence.ScheduledAt,
		occurrence.Status,
		occurrence.LastAttempt,
		occurrence.AttemptCount,
		occurrence.CreatedAt,
	)
	if err != nil {
		log.Printf("Error creating occurrence: %v", err)
		return err
	}
	rows, _ := result.RowsAffected()
	log.Printf("Created occurrence: id=%s, rows=%d", occurrence.ID, rows)
	return nil
}

func (r *OccurrenceRepository) ListByEventID(ctx context.Context, eventID uuid.UUID) ([]models.Occurrence, error) {
	query := `SELECT * FROM occurrences WHERE event_id = $1 ORDER BY scheduled_at DESC`
	var occurrences []models.Occurrence
	err := r.db.SelectContext(ctx, &occurrences, query, eventID)
	return occurrences, err
}

func (r *OccurrenceRepository) Update(ctx context.Context, occurrence *models.Occurrence) error {
	query := `
		UPDATE occurrences SET
			scheduled_at = $1,
			status = $2,
			last_attempt = $3,
			attempt_count = $4,
			updated_at = $5
		WHERE id = $6`

	_, err := r.db.ExecContext(ctx, query,
		occurrence.ScheduledAt,
		occurrence.Status,
		occurrence.LastAttempt,
		occurrence.AttemptCount,
		time.Now(),
		occurrence.ID,
	)
	return err
}

// ExistsAtTime checks if an occurrence exists for a given event at a specific time
func (r *OccurrenceRepository) ExistsAtTime(ctx context.Context, eventID uuid.UUID, scheduledAt time.Time) (bool, error) {
	log.Printf("Checking occurrence existence: event_id=%s, scheduled_at=%v", eventID, scheduledAt)
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM occurrences 
			WHERE event_id = $1 AND scheduled_at = $2
		)`
	err := r.db.GetContext(ctx, &exists, query, eventID, scheduledAt)
	if err != nil {
		log.Printf("Error checking occurrence existence: %v", err)
		return false, err
	}
	log.Printf("Occurrence exists=%v for event_id=%s at time=%v", exists, eventID, scheduledAt)
	return exists, err
}
