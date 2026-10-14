package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Type aliases for ledger domain primitives
type (
	LedgerEntryID string
	AccountType   string
)

// LedgerEntry represents a double-entry accounting record.
// Pure domain entity - no framework dependencies (RULE-ARCH-01).
type LedgerEntry struct {
	ID        LedgerEntryID
	InvoiceID InvoiceID
	TenantID  TenantID
	Account   AccountType // E.g., 'AccountsReceivable', 'Revenue'
	Debit     decimal.Decimal
	Credit    decimal.Decimal
	CreatedAt time.Time
}

// LedgerRepository defines the persistence contract for ledger entries.
// Abstract interface - implementations reside in infrastructure layer (RULE-ARCH-02).
type LedgerRepository interface {
	CreateLedgerEntries(ctx context.Context, entries []LedgerEntry) error
}
