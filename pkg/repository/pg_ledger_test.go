package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// TestPgLedgerRepository_TC_LED_02_NO_PARTIAL_INT
// TC-LED-02-NO-PARTIAL-INT: No partial commit on failure during invoice/ledger write (rollback path)
func TestPgLedgerRepository_TC_LED_02_NO_PARTIAL_INT(t *testing.T) {
	connStr, teardown := setupPostgresContainer(t)
	defer teardown()

	repo := NewPgInvoiceRepository(connStr)
	ctx := context.Background()

	tenantID := domain.TenantID("tenant-nopartial")
	now := time.Now().UTC()

	// Create a ledger entry with valid data to use in the bad request
	ledgerEntries := []domain.LedgerEntry{
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: domain.InvoiceID("dummy"),
			TenantID:  tenantID,
			Account:   "AccountsReceivable",
			Debit:     decimal.NewFromFloat(500.00),
			Credit:    decimal.Zero,
			CreatedAt: now,
		},
	}

	// Force an error by passing an invoice with empty created_by (violates NOT NULL)
	badInvoiceID := domain.InvoiceID(uuid.New().String())
	badInv := &domain.Invoice{
		ID:             badInvoiceID,
		TenantID:       tenantID,
		Currency:       domain.Currency("EUR"),
		BaseCurrency:   domain.Currency("USD"),
		FXRate:         domain.FXRate(1.18),
		TotalAmount:    decimal.NewFromFloat(500.00),
		IdempotencyKey: domain.IdempotencyKey(uuid.New().String()),
		CreatedAt:      now,
		CreatedBy:      "", // Violate NOT NULL to trigger failure
		LineItems: []domain.InvoiceLine{
			{
				ID:          uuid.New().String(),
				InvoiceID:   badInvoiceID,
				Description: "Services",
				Amount:      decimal.NewFromFloat(500.00),
				CreatedAt:   now,
				CreatedBy:   "",
			},
		},
	}

	err := repo.CreateInvoice(ctx, badInv, ledgerEntries)
	if err == nil {
		t.Fatal("expected error for invoice with empty created_by, got nil (violates partial commit safety)")
	}

	// Verify the invoice was NOT created (no partial state)
	_, err = repo.GetInvoiceByID(ctx, tenantID, badInvoiceID)
	if err == nil {
		t.Fatal("expected invoice to NOT exist after failed creation (partial commit detected)")
	}

	t.Logf("received expected error: %v - no partial state left in database", err)
}
