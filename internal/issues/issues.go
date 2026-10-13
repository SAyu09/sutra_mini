package issues

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sutra-mini/internal/apperr"
)

type Party struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type SKU struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type Issue struct {
	ID          int64 `json:"id"`
	Party       Party `json:"party"`
	SKU         SKU   `json:"sku"`
	Issued      int   `json:"issued"`
	Received    int   `json:"received"`
	Outstanding int   `json:"outstanding"`
	RatePaise   int64 `json:"rate_paise"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GetIssue fetches the issue read model with party, sku, and outstanding math.
func (s *Service) GetIssue(ctx context.Context, id int64) (*Issue, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: issue id must be positive", apperr.ErrInvalidInput)
	}

	query := `
		SELECT j.id, j.qty, j.received_qty, j.rate_paise,
		       p.id, p.name,
		       s.id, s.code, s.name
		FROM jobwork_issues j
		JOIN parties p ON p.id = j.party_id
		JOIN skus s ON s.id = j.sku_id
		WHERE j.id = $1
	`

	var issue Issue
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&issue.ID, &issue.Issued, &issue.Received, &issue.RatePaise,
		&issue.Party.ID, &issue.Party.Name,
		&issue.SKU.ID, &issue.SKU.Code, &issue.SKU.Name,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: issue %d not found", apperr.ErrNotFound, id)
		}
		return nil, fmt.Errorf("get issue: %w", err)
	}

	issue.Outstanding = issue.Issued - issue.Received
	return &issue, nil
}

// ListIssues fetches all jobwork issues with their parties and SKUs.
func (s *Service) ListIssues(ctx context.Context) ([]*Issue, error) {
	query := `
		SELECT j.id, j.qty, j.received_qty, j.rate_paise,
		       p.id, p.name,
		       s.id, s.code, s.name
		FROM jobwork_issues j
		JOIN parties p ON p.id = j.party_id
		JOIN skus s ON s.id = j.sku_id
		ORDER BY j.id ASC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	var result []*Issue
	for rows.Next() {
		var issue Issue
		err := rows.Scan(
			&issue.ID, &issue.Issued, &issue.Received, &issue.RatePaise,
			&issue.Party.ID, &issue.Party.Name,
			&issue.SKU.ID, &issue.SKU.Code, &issue.SKU.Name,
		)
		if err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		issue.Outstanding = issue.Issued - issue.Received
		result = append(result, &issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}

	return result, nil
}

