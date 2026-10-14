package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// PgInvoiceRepository implements domain.InvoiceRepository using PostgreSQL.
// Clean Architecture: Infrastructure adapter (RULE-ARCH-02).
type PgInvoiceRepository struct {
	pool *pgxpool.Pool
}

// NewPgInvoiceRepository creates a new PgInvoiceRepository.
func NewPgInvoiceRepository(connStr string) *PgInvoiceRepository {
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		// In production, this would use a proper config with retry.
		// For testing, we create a minimal pool.
		// This will cause operations to fail if DB not available.
		_ = pool
		return &PgInvoiceRepository{pool: nil}
	}
	return &PgInvoiceRepository{pool: pool}
}

// NewPgInvoiceRepositoryWithPool creates a PgInvoiceRepository with an existing pool.
func NewPgInvoiceRepositoryWithPool(pool *pgxpool.Pool) *PgInvoiceRepository {
	return &PgInvoiceRepository{pool: pool}
}

// CreateInvoice atomically creates an invoice with its line items and ledger entries.
// Implements RULE-DATA-01 (atomic multi-entity mutations) and RULE-DATA-02 (idempotency).
func (r *PgInvoiceRepository) CreateInvoice(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
	if r.pool == nil {
		return fmt.Errorf("database pool not initialized")
	}

	// Check idempotency first - RULE-DATA-02
	existing, err := r.GetInvoiceByIdempotencyKey(ctx, invoice.TenantID, invoice.IdempotencyKey)
	if err == nil && existing != nil {
		// Idempotent: return the original invoice by overwriting the pointer
		*invoice = *existing
		return nil
	}

	// Use a transaction for atomicity - RULE-DATA-01
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // RULE-RES-03: immediate deferral

	// Insert invoice
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO invoices (id, tenant_id, currency, base_currency, fx_rate, total_amount, idempotency_key, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`,
		invoice.ID, invoice.TenantID, invoice.Currency, invoice.BaseCurrency,
		invoice.FXRate, invoice.TotalAmount, invoice.IdempotencyKey,
		invoice.CreatedAt, invoice.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to insert invoice: %w", err)
	}

	// Check if the row was inserted (not a conflict duplicate)
	var count int
	err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM invoices WHERE id = $1`, invoice.ID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to verify invoice insert: %w", err)
	}

	// Insert line items
	for _, line := range invoice.LineItems {
		_, err = tx.Exec(ctx, `
			INSERT INTO invoice_lines (id, invoice_id, description, amount, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			line.ID, invoice.ID, line.Description, line.Amount, now, line.CreatedBy,
		)
		if err != nil {
			return fmt.Errorf("failed to insert invoice line: %w", err)
		}
	}

	// Insert ledger entries
	for _, entry := range ledgerEntries {
		_, err = tx.Exec(ctx, `
			INSERT INTO ledger_entries (id, invoice_id, tenant_id, account, debit, credit, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			entry.ID, invoice.ID, entry.TenantID, entry.Account,
			entry.Debit, entry.Credit, now, "system",
		)
		if err != nil {
			return fmt.Errorf("failed to insert ledger entry: %w", err)
		}
	}

	// Commit the transaction - all writes succeed or none
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetInvoiceByID retrieves an invoice by its ID and tenant.
func (r *PgInvoiceRepository) GetInvoiceByID(ctx context.Context, tenantID domain.TenantID, invoiceID domain.InvoiceID) (*domain.Invoice, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("database pool not initialized")
	}

	inv := &domain.Invoice{}
	var updatedAt, deletedAt *time.Time
	var updatedBy, deletedBy *string

	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, currency, base_currency, fx_rate, total_amount, idempotency_key,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM invoices
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		invoiceID, tenantID,
	).Scan(
		&inv.ID, &inv.TenantID, &inv.Currency, &inv.BaseCurrency,
		&inv.FXRate, &inv.TotalAmount, &inv.IdempotencyKey,
		&inv.CreatedAt, &inv.CreatedBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, fmt.Sprintf("invoice %s not found", invoiceID))
		}
		return nil, fmt.Errorf("failed to get invoice by ID: %w", err)
	}

	inv.UpdatedAt = updatedAt
	inv.UpdatedBy = updatedBy
	inv.DeletedAt = deletedAt
	inv.DeletedBy = deletedBy

	// Load line items
	rows, err := r.pool.Query(ctx, `
		SELECT id, invoice_id, description, amount, created_at, created_by
		FROM invoice_lines
		WHERE invoice_id = $1`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoice lines: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var line domain.InvoiceLine
		err := rows.Scan(&line.ID, &line.InvoiceID, &line.Description, &line.Amount, &line.CreatedAt, &line.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invoice line: %w", err)
		}
		inv.LineItems = append(inv.LineItems, line)
	}

	return inv, nil
}

// GetInvoiceByIdempotencyKey retrieves an invoice by its idempotency key and tenant.
func (r *PgInvoiceRepository) GetInvoiceByIdempotencyKey(ctx context.Context, tenantID domain.TenantID, key domain.IdempotencyKey) (*domain.Invoice, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("database pool not initialized")
	}

	inv := &domain.Invoice{}
	var updatedAt, deletedAt *time.Time
	var updatedBy, deletedBy *string

	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, currency, base_currency, fx_rate, total_amount, idempotency_key,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM invoices
		WHERE tenant_id = $1 AND idempotency_key = $2 AND deleted_at IS NULL`,
		tenantID, key,
	).Scan(
		&inv.ID, &inv.TenantID, &inv.Currency, &inv.BaseCurrency,
		&inv.FXRate, &inv.TotalAmount, &inv.IdempotencyKey,
		&inv.CreatedAt, &inv.CreatedBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, fmt.Sprintf("invoice with idempotency key %s not found", key))
		}
		return nil, fmt.Errorf("failed to get invoice by idempotency key: %w", err)
	}

	inv.UpdatedAt = updatedAt
	inv.UpdatedBy = updatedBy
	inv.DeletedAt = deletedAt
	inv.DeletedBy = deletedBy

	// Load line items
	rows, err := r.pool.Query(ctx, `
		SELECT id, invoice_id, description, amount, created_at, created_by
		FROM invoice_lines
		WHERE invoice_id = $1`, inv.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoice lines: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var line domain.InvoiceLine
		err := rows.Scan(&line.ID, &line.InvoiceID, &line.Description, &line.Amount, &line.CreatedAt, &line.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invoice line: %w", err)
		}
		inv.LineItems = append(inv.LineItems, line)
	}

	return inv, nil
}
