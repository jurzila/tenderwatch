package main

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

const upsertNoticeSQL = `
INSERT INTO NOTICES (id, heading, type, status, issue_date, deadline, estimated_amount, currency)
VALUES ($1, $2, $3, $4, $5, $6 ,$7 ,$8)
ON CONFLICT (id) DO UPDATE SET heading = EXCLUDED.heading, type = EXCLUDED.type, status = EXCLUDED.status, issue_date = EXCLUDED.issue_date, deadline = EXCLUDED.deadline, estimated_amount = EXCLUDED.estimated_amount, currency = EXCLUDED.currency`

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
