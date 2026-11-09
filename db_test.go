//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := context.Background()
	pool, err := connectDB(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := applySchema(ctx, pool); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return pool
}

func TestSaveNotice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	t.Cleanup(func() { deleteRows(t, pool, `DELETE FROM buyers WHERE organization_id = $1`, "test-org-1") })
	t.Cleanup(func() { deleteRows(t, pool, `DELETE FROM buyers WHERE organization_id = $1`, "test-org-2") })
	t.Cleanup(func() { deleteRows(t, pool, `DELETE FROM notices WHERE id = $1`, "test-1") })
	t.Cleanup(func() { deleteRows(t, pool, `DELETE FROM notices WHERE id = $1`, "test-2") })

	n := notice{
		ID:      "test-1",
		Heading: "Test heading",
		Buyers: []buyer{
			{OrganizationID: "test-org-1", Name: "TestBuyer"},
		},
		Status:         "pending",
		IssueDate:      time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC),
		Deadline:       time.Date(2026, time.January, 31, 16, 0, 0, 0, time.UTC),
		EstimatedValue: &money{CurrencyCode: "NOK", Amount: 10000},
		CPVCodes:       []string{"45000000", "25800000"},
		Type:           "testType",
	}

	if err := saveNotice(ctx, pool, n); err != nil {
		t.Fatalf("first notice first save: %v", err)
	}
	if err := saveNotice(ctx, pool, n); err != nil {
		t.Fatalf("first notice second save: %v", err)
	}

	if got := countRows(t, pool, `SELECT count(*) FROM notices WHERE id = $1`, n.ID); got != 1 {
		t.Errorf("notices: got %d rows, want 1", got)
	}

	if got := countRows(t, pool, `SELECT count(*) FROM notice_cpv_codes WHERE notice_id = $1`, n.ID); got != 2 {
		t.Errorf("cpv codes: got %d rows, want 2", got)
	}

	n.CPVCodes = []string{"45000000"}
	if err := saveNotice(ctx, pool, n); err != nil {
		t.Fatalf("save with fewer codes: %v", err)
	}

	if got := countRows(t, pool, `SELECT count(*) FROM notice_cpv_codes WHERE notice_id = $1`, n.ID); got != 1 {
		t.Fatalf("cpv codes after removal: got %d rows, want 1", got)
	}

	var code string
	if err := pool.QueryRow(ctx, `SELECT cpv_code FROM notice_cpv_codes WHERE notice_id = $1`, n.ID).Scan(&code); err != nil {
		t.Fatalf("read cpv code: %v", err)
	}
	if code != "45000000" {
		t.Errorf("cpv code: got %q, want %q", code, "45000000")
	}

}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	return n
}

func deleteRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Errorf("delete query failed: %v", err)
	}
}
