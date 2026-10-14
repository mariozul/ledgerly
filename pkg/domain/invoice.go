package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Type aliases for domain primitives
type (
	InvoiceID       string
	TenantID        string
	IdempotencyKey  string
	Currency        string
	FXRate          float64
)

// Invoice represents a multi-currency invoice.
// Pure domain entity - no framework dependencies (RULE-ARCH-01).
type Invoice struct {
	ID             InvoiceID
	TenantID       TenantID
	Currency       Currency
	BaseCurrency   Currency
	FXRate         FXRate // Immutable snapshot after creation (FR-FX-01, AC-03)
	TotalAmount    decimal.Decimal
	LineItems      []InvoiceLine
	IdempotencyKey IdempotencyKey
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      *time.Time
	UpdatedBy      *string
	DeletedAt      *time.Time
	DeletedBy      *string
}

// InvoiceLine represents a single line item on an invoice.
type InvoiceLine struct {
	ID          string
	InvoiceID   InvoiceID
	Description string
	Amount      decimal.Decimal
	CreatedAt   time.Time
	CreatedBy   string
}

// InvoiceRepository defines the persistence contract for invoices.
// Abstract interface - implementations reside in infrastructure layer (RULE-ARCH-02).
type InvoiceRepository interface {
	CreateInvoice(ctx context.Context, invoice *Invoice, ledgerEntries []LedgerEntry) error
	GetInvoiceByID(ctx context.Context, tenantID TenantID, invoiceID InvoiceID) (*Invoice, error)
	GetInvoiceByIdempotencyKey(ctx context.Context, tenantID TenantID, key IdempotencyKey) (*Invoice, error)
}
