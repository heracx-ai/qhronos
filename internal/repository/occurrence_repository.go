package repository

import (
	"context"
	"database/sql"
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

func (r *OccurrenceRepository) GetByID(ctx context.Context, id int) (*models.Occurrence, error) {
	log.Printf("Getting occurrence by ID: %d", id)
	var occurrence models.Occurrence
	query := `SELECT * FROM occurrences WHERE id = $1`
	err := r.db.GetContext(ctx, &occurrence, query, id)
	if err == sql.ErrNoRows {
		log.Printf("Occurrence not found: %d", id)
		return nil, nil
	}
	if err != nil {
		log.Printf("Error getting occurrence %d: %v", id, err)
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
		ID           int       `db:"id"`
		OccurrenceID uuid.UUID `db:"occurrence_id"`
		EventID      uuid.UUID `db:"event_id"`
		ScheduledAt  time.Time `db:"scheduled_at"`
		Status       string    `db:"status"`
		AttemptCount int       `db:"attempt_count"`
		Timestamp    time.Time `db:"timestamp"`
		StatusCode   int       `db:"status_code"`
		ResponseBody string    `db:"response_body"`
		ErrorMessage string    `db:"error_message"`
		StartedAt    time.Time `db:"started_at"`
		CompletedAt  time.Time `db:"completed_at"`
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
			OccurrenceID: tempOccurrence.OccurrenceID,
			EventID:      tempOccurrence.EventID,
			ScheduledAt:  tempOccurrence.ScheduledAt,
			Status:       models.OccurrenceStatus(tempOccurrence.Status),
			AttemptCount: tempOccurrence.AttemptCount,
			Timestamp:    tempOccurrence.Timestamp,
			StatusCode:   tempOccurrence.StatusCode,
			ResponseBody: tempOccurrence.ResponseBody,
			ErrorMessage: tempOccurrence.ErrorMessage,
			StartedAt:    tempOccurrence.StartedAt,
			CompletedAt:  tempOccurrence.CompletedAt,
		}
	}

	return occurrences, total, nil
}

func (r *OccurrenceRepository) Create(ctx context.Context, occurrence *models.Occurrence) error {
	log.Printf("Creating occurrence: occurrence_id=%s, event_id=%s, scheduled_at=%v",
		occurrence.OccurrenceID, occurrence.EventID, occurrence.ScheduledAt)
	query := `
		INSERT INTO occurrences (occurrence_id, event_id, scheduled_at, status, attempt_count, timestamp, status_code, response_body, error_message, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, timestamp`

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
		log.Printf("Error creating occurrence: %v", err)
		return err
	}
	log.Printf("Created occurrence: id=%d", occurrence.ID)
	return nil
}

func (r *OccurrenceRepository) ListByEventID(ctx context.Context, eventID uuid.UUID) ([]models.Occurrence, error) {
	query := `SELECT * FROM occurrences WHERE event_id = $1 ORDER BY scheduled_at DESC`
	var tempOccurrences []struct {
		ID           int       `db:"id"`
		OccurrenceID uuid.UUID `db:"occurrence_id"`
		EventID      uuid.UUID `db:"event_id"`
		ScheduledAt  time.Time `db:"scheduled_at"`
		Status       string    `db:"status"`
		AttemptCount int       `db:"attempt_count"`
		Timestamp    time.Time `db:"timestamp"`
		StatusCode   int       `db:"status_code"`
		ResponseBody string    `db:"response_body"`
		ErrorMessage string    `db:"error_message"`
		StartedAt    time.Time `db:"started_at"`
		CompletedAt  time.Time `db:"completed_at"`
	}

	err := r.db.SelectContext(ctx, &tempOccurrences, query, eventID)
	if err != nil {
		return nil, err
	}
	occurrences := make([]models.Occurrence, len(tempOccurrences))
	for i, tempOccurrence := range tempOccurrences {
		occurrences[i] = models.Occurrence{
			ID:           tempOccurrence.ID,
			OccurrenceID: tempOccurrence.OccurrenceID,
			EventID:      tempOccurrence.EventID,
			ScheduledAt:  tempOccurrence.ScheduledAt,
			Status:       models.OccurrenceStatus(tempOccurrence.Status),
			AttemptCount: tempOccurrence.AttemptCount,
			Timestamp:    tempOccurrence.Timestamp,
			StatusCode:   tempOccurrence.StatusCode,
			ResponseBody: tempOccurrence.ResponseBody,
			ErrorMessage: tempOccurrence.ErrorMessage,
			StartedAt:    tempOccurrence.StartedAt,
			CompletedAt:  tempOccurrence.CompletedAt,
		}
	}
	return occurrences, nil
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

// GetLatestByOccurrenceID fetches the latest occurrence row for a given OccurrenceID
func (r *OccurrenceRepository) GetLatestByOccurrenceID(ctx context.Context, occurrenceID uuid.UUID) (*models.Occurrence, error) {
	var occurrence models.Occurrence
	query := `SELECT * FROM occurrences WHERE occurrence_id = $1 ORDER BY id DESC LIMIT 1`
	err := r.db.GetContext(ctx, &occurrence, query, occurrenceID)
	if err != nil {
		return nil, err
	}
	return &occurrence, nil
}
