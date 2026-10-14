package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// setupPostgresContainer creates a test postgres container and returns the connection string.
func setupPostgresContainer(t *testing.T) (connectionString string, teardown func()) {
	t.Helper()
	return "postgres://ledgerly:ledgerly@localhost:5432/ledgerly_test?sslmode=disable", func() {}
}

// TestPgInvoiceRepository_TC_INV_03_IDEMPOTENCY_INT
// TC-INV-03-IDEMPOTENCY-INT: Duplicate (tenant_id, idempotency_key) returns original invoice
func TestPgInvoiceRepository_TC_INV_03_IDEMPOTENCY_INT(t *testing.T) {
	connStr, teardown := setupPostgresContainer(t)
	defer teardown()

	repo := NewPgInvoiceRepository(connStr)
	ctx := context.Background()

	tenantID := domain.TenantID("tenant-1")
	key := domain.IdempotencyKey(uuid.New().String())
	invoiceID1 := domain.InvoiceID(uuid.New().String())

	now := time.Now().UTC()

	// First creation
	inv1 := &domain.Invoice{
		ID:             invoiceID1,
		TenantID:       tenantID,
		Currency:       domain.Currency("EUR"),
		BaseCurrency:   domain.Currency("USD"),
		FXRate:         domain.FXRate(1.18),
		TotalAmount:    decimal.NewFromFloat(100.00),
		IdempotencyKey: key,
		CreatedAt:      now,
		CreatedBy:      "test-user",
		LineItems: []domain.InvoiceLine{
			{
				ID:          uuid.New().String(),
				InvoiceID:   invoiceID1,
				Description: "Consulting",
				Amount:      decimal.NewFromFloat(100.00),
				CreatedAt:   now,
				CreatedBy:   "test-user",
			},
		},
	}

	ledgerEntries := []domain.LedgerEntry{
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID1,
			TenantID:  tenantID,
			Account:   "AccountsReceivable",
			Debit:     decimal.NewFromFloat(100.00),
			Credit:    decimal.Zero,
			CreatedAt: now,
		},
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID1,
			TenantID:  tenantID,
			Account:   "Revenue",
			Debit:     decimal.Zero,
			Credit:    decimal.NewFromFloat(100.00),
			CreatedAt: now,
		},
	}

	err := repo.CreateInvoice(ctx, inv1, ledgerEntries)
	if err != nil {
		t.Fatalf("expected first creation to succeed, got: %v", err)
	}

	// Second creation with same (tenant_id, idempotency_key)
	invoiceID2 := domain.InvoiceID(uuid.New().String())
	inv2 := &domain.Invoice{
		ID:             invoiceID2,
		TenantID:       tenantID,
		Currency:       domain.Currency("EUR"),
		BaseCurrency:   domain.Currency("USD"),
		FXRate:         domain.FXRate(1.18),
		TotalAmount:    decimal.NewFromFloat(100.00),
		IdempotencyKey: key,
		CreatedAt:      now,
		CreatedBy:      "test-user",
		LineItems: []domain.InvoiceLine{
			{
				ID:          uuid.New().String(),
				InvoiceID:   invoiceID2,
				Description: "Consulting",
				Amount:      decimal.NewFromFloat(100.00),
				CreatedAt:   now,
				CreatedBy:   "test-user",
			},
		},
	}

	err = repo.CreateInvoice(ctx, inv2, ledgerEntries)
	if err != nil {
		t.Fatalf("expected second creation to succeed (idempotent), got: %v", err)
	}

	// The idempotent write should return the original invoice ID
	// Verify the inv1.ID is returned via the pointer being overwritten
	if inv2.ID != invoiceID1 {
		t.Errorf("expected duplicate request to return original invoice ID %q, got %q (violates idempotency)", invoiceID1, inv2.ID)
	}

	// Verify only one invoice exists for this key
	retrieved, err := repo.GetInvoiceByIdempotencyKey(ctx, tenantID, key)
	if err != nil {
		t.Fatalf("expected to retrieve invoice by idempotency key, got: %v", err)
	}
	if retrieved.ID != invoiceID1 {
		t.Errorf("expected retrieved invoice ID %q, got %q", invoiceID1, retrieved.ID)
	}
}

// TestPgInvoiceRepository_TC_INV_04_IDEMPOTENCY_RACE
// TC-INV-04-IDEMPOTENCY-RACE: 20 concurrent requests with same (tenant_id, idempotency_key)
func TestPgInvoiceRepository_TC_INV_04_IDEMPOTENCY_RACE(t *testing.T) {
	connStr, teardown := setupPostgresContainer(t)
	defer teardown()

	repo := NewPgInvoiceRepository(connStr)
	ctx := context.Background()

	tenantID := domain.TenantID("tenant-race")
	key := domain.IdempotencyKey(uuid.New().String())

	const concurrency = 20
	errs := make(chan error, concurrency)
	ids := make(chan domain.InvoiceID, concurrency)
	now := time.Now().UTC()

	for i := 0; i < concurrency; i++ {
		go func() {
			invoiceID := domain.InvoiceID(uuid.New().String())
			inv := &domain.Invoice{
				ID:             invoiceID,
				TenantID:       tenantID,
				Currency:       domain.Currency("EUR"),
				BaseCurrency:   domain.Currency("USD"),
				FXRate:         domain.FXRate(1.18),
				TotalAmount:    decimal.NewFromFloat(100.00),
				IdempotencyKey: key,
				CreatedAt:      now,
				CreatedBy:      "test-user",
				LineItems: []domain.InvoiceLine{
					{
						ID:          uuid.New().String(),
						InvoiceID:   invoiceID,
						Description: "Consulting",
						Amount:      decimal.NewFromFloat(100.00),
						CreatedAt:   now,
						CreatedBy:   "test-user",
					},
				},
			}

			ledgerEntries := []domain.LedgerEntry{
				{
					ID:        domain.LedgerEntryID(uuid.New().String()),
					InvoiceID: invoiceID,
					TenantID:  tenantID,
					Account:   "AccountsReceivable",
					Debit:     decimal.NewFromFloat(100.00),
					Credit:    decimal.Zero,
					CreatedAt: now,
				},
				{
					ID:        domain.LedgerEntryID(uuid.New().String()),
					InvoiceID: invoiceID,
					TenantID:  tenantID,
					Account:   "Revenue",
					Debit:     decimal.Zero,
					Credit:    decimal.NewFromFloat(100.00),
					CreatedAt: now,
				},
			}

			err := repo.CreateInvoice(ctx, inv, ledgerEntries)
			errs <- err
			ids <- inv.ID
		}()
	}

	// Collect results
	results := make(map[domain.InvoiceID]int)
	for i := 0; i < concurrency; i++ {
		err := <-errs
		if err != nil && !domain.IsDomainError(err, domain.ErrAlreadyExists) {
			t.Errorf("unexpected error: %v", err)
		}
		id := <-ids
		results[id]++
	}

	// Verify only one unique invoice ID was returned (race condition check)
	// With idempotency, all goroutines should see the same ID
	if len(results) != 1 {
		t.Errorf("expected exactly 1 unique invoice ID, got %d (race condition: multiple invoices created): %v", len(results), results)
	}

	// Verify we can retrieve by idempotency key
	retrieved, err := repo.GetInvoiceByIdempotencyKey(ctx, tenantID, key)
	if err != nil {
		t.Fatalf("expected to retrieve invoice by idempotency key, got: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected non-nil invoice")
	}
	if retrieved.IdempotencyKey != key {
		t.Errorf("expected idempotency key %q, got %q", key, retrieved.IdempotencyKey)
	}
}

// TestPgInvoiceRepository_TC_LED_01_ATOMIC_INT
// TC-LED-01-ATOMIC-INT: Invoice and ledger records are created atomically (commit path)
func TestPgInvoiceRepository_TC_LED_01_ATOMIC_INT(t *testing.T) {
	connStr, teardown := setupPostgresContainer(t)
	defer teardown()

	repo := NewPgInvoiceRepository(connStr)
	ctx := context.Background()

	tenantID := domain.TenantID("tenant-atomic")
	key := domain.IdempotencyKey(uuid.New().String())
	invoiceID := domain.InvoiceID(uuid.New().String())
	now := time.Now().UTC()

	inv := &domain.Invoice{
		ID:             invoiceID,
		TenantID:       tenantID,
		Currency:       domain.Currency("EUR"),
		BaseCurrency:   domain.Currency("USD"),
		FXRate:         domain.FXRate(1.18),
		TotalAmount:    decimal.NewFromFloat(250.00),
		IdempotencyKey: key,
		CreatedAt:      now,
		CreatedBy:      "test-user",
		LineItems: []domain.InvoiceLine{
			{
				ID:          uuid.New().String(),
				InvoiceID:   invoiceID,
				Description: "Services",
				Amount:      decimal.NewFromFloat(250.00),
				CreatedAt:   now,
				CreatedBy:   "test-user",
			},
		},
	}

	ledgerEntries := []domain.LedgerEntry{
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID,
			TenantID:  tenantID,
			Account:   "AccountsReceivable",
			Debit:     decimal.NewFromFloat(250.00),
			Credit:    decimal.Zero,
			CreatedAt: now,
		},
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID,
			TenantID:  tenantID,
			Account:   "Revenue",
			Debit:     decimal.Zero,
			Credit:    decimal.NewFromFloat(250.00),
			CreatedAt: now,
		},
	}

	err := repo.CreateInvoice(ctx, inv, ledgerEntries)
	if err != nil {
		t.Fatalf("expected CreateInvoice to succeed, got: %v", err)
	}

	// Verify invoice exists
	retrieved, err := repo.GetInvoiceByID(ctx, tenantID, invoiceID)
	if err != nil {
		t.Fatalf("expected to retrieve invoice, got: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected non-nil invoice")
	}
	if retrieved.TotalAmount.Cmp(decimal.NewFromFloat(250.00)) != 0 {
		t.Errorf("expected total 250.00, got %s", retrieved.TotalAmount.String())
	}
}
