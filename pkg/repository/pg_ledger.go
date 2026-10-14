package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// PgLedgerRepository implements domain.LedgerRepository using PostgreSQL.
// Clean Architecture: Infrastructure adapter (RULE-ARCH-02).
type PgLedgerRepository struct {
	pool *pgxpool.Pool
}

// NewPgLedgerRepository creates a new PgLedgerRepository.
func NewPgLedgerRepository(connStr string) *PgLedgerRepository {
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return &PgLedgerRepository{pool: nil}
	}
	return &PgLedgerRepository{pool: pool}
}

// NewPgLedgerRepositoryWithPool creates a PgLedgerRepository with an existing pool.
func NewPgLedgerRepositoryWithPool(pool *pgxpool.Pool) *PgLedgerRepository {
	return &PgLedgerRepository{pool: pool}
}

// CreateLedgerEntries inserts multiple ledger entries within a transaction.
// Implemented as part of the atomic invoice+ledger write pattern (RULE-DATA-01).
// This standalone method assumes the caller manages the transaction.
func (r *PgLedgerRepository) CreateLedgerEntries(ctx context.Context, entries []domain.LedgerEntry) error {
	if r.pool == nil {
		return fmt.Errorf("database pool not initialized")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // RULE-RES-03

	now := time.Now().UTC()
	for _, entry := range entries {
		_, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (id, invoice_id, tenant_id, account, debit, credit, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			entry.ID, entry.InvoiceID, entry.TenantID, entry.Account,
			entry.Debit, entry.Credit, now, "system",
		)
		if err != nil {
			return fmt.Errorf("failed to insert ledger entry: %w", err)
		}
	}

	return tx.Commit(ctx)
}
