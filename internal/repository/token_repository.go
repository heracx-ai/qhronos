package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/feedloop/qhronos/internal/models"
)

type TokenRepository struct {
	db *sqlx.DB
}

func NewTokenRepository(db *sqlx.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

func (r *TokenRepository) Create(ctx context.Context, token *models.Token) error {
	query := `
		INSERT INTO tokens (
			sub, access, scope, expires_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5
		)`

	_, err := r.db.ExecContext(ctx, query,
		token.Sub,
		token.Access,
		pq.Array(token.Scope),
		token.ExpiresAt,
		time.Now(),
	)
	return err
}

func (r *TokenRepository) GetByID(ctx context.Context, id string) (*models.Token, error) {
	query := `
		SELECT sub, access, scope, expires_at, created_at
		FROM tokens
		WHERE sub = $1`

	var token models.Token
	err := r.db.GetContext(ctx, &token, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *TokenRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM tokens WHERE sub = $1`
	result, err := r.db.ExecContext(ctx, query, id)
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

func (r *TokenRepository) ListBySub(ctx context.Context, sub string) ([]*models.Token, error) {
	query := `
		SELECT sub, access, scope, expires_at, created_at
		FROM tokens
		WHERE sub = $1
		ORDER BY created_at DESC`

	var tokens []*models.Token
	err := r.db.SelectContext(ctx, &tokens, query, sub)
	if err != nil {
		return nil, err
	}
	return tokens, nil
} 