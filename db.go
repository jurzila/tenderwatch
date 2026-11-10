package main

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

const upsertNoticeSQL = `
INSERT INTO NOTICES (id, heading, type, status, issue_date, deadline, estimated_amount, currency)
VALUES ($1, $2, $3, $4, $5, $6 ,$7 ,$8)
ON CONFLICT (id) DO UPDATE SET heading = EXCLUDED.heading, type = EXCLUDED.type, status = EXCLUDED.status, issue_date = EXCLUDED.issue_date, deadline = EXCLUDED.deadline, estimated_amount = EXCLUDED.estimated_amount, currency = EXCLUDED.currency`

type noticeSummary struct {
	ID              string     `json:"id"`
	Heading         string     `json:"heading"`
	Deadline        *time.Time `json:"deadline"`
	EstimatedAmount *float64   `json:"estimatedAmount" db:"estimated_amount"`
	Currency        *string    `json:"currency"`
}

type buyerTotal struct {
	OrganizationID string   `json:"organizationId" db:"organization_id"`
	Name           string   `json:"name"`
	TotalEstimated *float64 `json:"totalEstimated" db:"total_estimated"`
	NoticeCount    int     `json:"noticeCount" db:"total_notices"`
}

type pgStore struct {
	pool *pgxpool.Pool
}

func connectDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
func applySchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullIfZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func saveNotice(ctx context.Context, pool *pgxpool.Pool, n notice) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var amount *float64
	var currency *string

	if n.EstimatedValue != nil {
		amount = &n.EstimatedValue.Amount
		currency = &n.EstimatedValue.CurrencyCode
	}

	if _, err = tx.Exec(ctx, upsertNoticeSQL, n.ID, n.Heading, n.Type, nullIfEmpty(n.Status), n.IssueDate, nullIfZero(n.Deadline), amount, currency); err != nil {
		return fmt.Errorf("upsert notices: %w", err)
	}

	if _, err = tx.Exec(ctx, `DELETE FROM notice_buyers WHERE notice_id = $1`, n.ID); err != nil {
		return fmt.Errorf("delete old buyer links: %w", err)
	}

	for _, b := range n.Buyers {
		if _, err := tx.Exec(ctx, `
	INSERT INTO buyers (organization_id, name)
	VALUES ($1, $2)
	ON CONFLICT (organization_id) DO UPDATE SET name = EXCLUDED.name`,
			b.OrganizationID, b.Name); err != nil {
			return fmt.Errorf("upsert buyer %s: %w", b.OrganizationID, err)
		}
		if _, err := tx.Exec(ctx, `
	INSERT INTO notice_buyers (notice_id, organization_id) VALUES ($1, $2)`,
			n.ID, b.OrganizationID); err != nil {
			return fmt.Errorf("upsert buyer %s for notice %s link: %w", b.OrganizationID, n.ID, err)
		}
	}

	if _, err = tx.Exec(ctx, `DELETE FROM notice_cpv_codes WHERE notice_id = $1`, n.ID); err != nil {
		return fmt.Errorf("delete old cpv codes links: %w", err)
	}

	for _, c := range n.CPVCodes {
		if _, err := tx.Exec(ctx, `
	INSERT INTO notice_cpv_codes (notice_id, cpv_code) VALUES ($1, $2)`,
			n.ID, c); err != nil {
			return fmt.Errorf("upsert cpv code %s for notice %s: %w", c, n.ID, err)
		}
	}

	return tx.Commit(ctx)
}

func (s *pgStore) NoticesByCPV(ctx context.Context, prefix string) ([]noticeSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT n.id, n.heading, n.deadline, n.estimated_amount, n.currency FROM notices n JOIN notice_cpv_codes ncc ON n.id = ncc.notice_id where ncc.cpv_code LIKE $1 || '%' ORDER BY n.deadline`, prefix)
	if err != nil {
		return nil, fmt.Errorf("query notices by cpv: %w", err)
	}

	notices, err := pgx.CollectRows(rows, pgx.RowToStructByName[noticeSummary])
	if err != nil {
		return nil, fmt.Errorf("scan notices: %w", err)
	}
	return notices, nil
}

func (s *pgStore) ClosingSoon(ctx context.Context, days int) ([]noticeSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT n.id, n.heading, n.deadline, n.estimated_amount, n.currency FROM notices n where n.deadline BETWEEN now() AND now() + make_interval(days => $1) ORDER BY n.deadline`, days)
	if err != nil {
		return nil, fmt.Errorf("query notices by days: %w", err)
	}

	notices, err := pgx.CollectRows(rows, pgx.RowToStructByName[noticeSummary])
	if err != nil {
		return nil, fmt.Errorf("scan notices: %w", err)
	}
	return notices, nil
}

func (s *pgStore) TopBuyers(ctx context.Context, limit int) ([]buyerTotal, error) {
	rows, err := s.pool.Query(ctx, `SELECT b.organization_id, b.name, SUM(n.estimated_amount) as total_estimated, COUNT(*) as total_notices
		FROM buyers b
		JOIN notice_buyers nb ON b.organization_id = nb.organization_id  
		JOIN notices n ON n.id = nb.notice_id
		GROUP BY b.organization_id, b.name
		ORDER BY total_estimated desc nulls last
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query notices by buyers: %w", err)
	}

	buyers, err := pgx.CollectRows(rows, pgx.RowToStructByName[buyerTotal])
	if err != nil {
		return nil, fmt.Errorf("scan buyers: %w", err)
	}
	return buyers, nil
}
