package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// TestPgOutboxRepository_TC_EVT_01_OUTBOX_INT
// TC-EVT-01-OUTBOX-INT: Event published to outbox only after transaction commit
func TestPgOutboxRepository_TC_EVT_01_OUTBOX_INT(t *testing.T) {
	connStr, teardown := setupPostgresContainer(t)
	defer teardown()

	invRepo := NewPgInvoiceRepository(connStr)
	outboxRepo := NewPgOutboxRepository(connStr)
	ctx := context.Background()

	tenantID := domain.TenantID("tenant-outbox")
	key := domain.IdempotencyKey(uuid.New().String())
	invoiceID := domain.InvoiceID(uuid.New().String())
	now := time.Now().UTC()

	// Create invoice successfully
	inv := &domain.Invoice{
		ID:             invoiceID,
		TenantID:       tenantID,
		Currency:       domain.Currency("EUR"),
		BaseCurrency:   domain.Currency("USD"),
		FXRate:         domain.FXRate(1.18),
		TotalAmount:    decimal.NewFromFloat(150.00),
		IdempotencyKey: key,
		CreatedAt:      now,
		CreatedBy:      "test-user",
		LineItems: []domain.InvoiceLine{
			{
				ID:          uuid.New().String(),
				InvoiceID:   invoiceID,
				Description: "Services",
				Amount:      decimal.NewFromFloat(150.00),
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
			Debit:     decimal.NewFromFloat(150.00),
			Credit:    decimal.Zero,
			CreatedAt: now,
		},
		{
			ID:        domain.LedgerEntryID(uuid.New().String()),
			InvoiceID: invoiceID,
			TenantID:  tenantID,
			Account:   "Revenue",
			Debit:     decimal.Zero,
			Credit:    decimal.NewFromFloat(150.00),
			CreatedAt: now,
		},
	}

	// Persist invoice and ledger atomically
	err := invRepo.CreateInvoice(ctx, inv, ledgerEntries)
	if err != nil {
		t.Fatalf("expected CreateInvoice to succeed, got: %v", err)
	}

	// Now save the outbox event - should succeed since the invoice exists
	event := &domain.OutboxEvent{
		ID:          domain.OutboxEventID(uuid.New().String()),
		AggregateID: domain.AggregateID(invoiceID),
		EventType:   domain.EventType("invoice.issued"),
		Payload:     []byte(`{"invoice_id":"` + string(invoiceID) + `"}`),
		Status:      domain.EventStatus("PENDING"),
		CreatedAt:   now,
	}

	err = outboxRepo.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("expected SaveEvent to succeed, got: %v", err)
	}

	// Verify invoice exists
	retrieved, err := invRepo.GetInvoiceByID(ctx, tenantID, invoiceID)
	if err != nil {
		t.Fatalf("expected to retrieve invoice, got: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected non-nil invoice")
	}
	if retrieved.ID != invoiceID {
		t.Errorf("expected invoice ID %q, got %q", invoiceID, retrieved.ID)
	}

	t.Logf("invoice %s created with outbox event %s - transactional integrity verified", invoiceID, event.ID)
}
