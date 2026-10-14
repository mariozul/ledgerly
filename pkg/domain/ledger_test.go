package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestLedgerEntry_Creation(t *testing.T) {
	t.Run("creates ledger entry with valid fields", func(t *testing.T) {
		now := time.Now().UTC()
		entry := &LedgerEntry{
			ID:        LedgerEntryID("ledger-001"),
			InvoiceID: InvoiceID("inv-001"),
			TenantID:  TenantID("tenant-1"),
			Account:   AccountType("AccountsReceivable"),
			Debit:     decimal.NewFromFloat(1000.00),
			Credit:    decimal.NewFromFloat(0),
			CreatedAt: now,
		}

		if entry.ID != "ledger-001" {
			t.Errorf("expected ID 'ledger-001', got %q", entry.ID)
		}
		if entry.TenantID != "tenant-1" {
			t.Errorf("expected TenantID 'tenant-1', got %q", entry.TenantID)
		}
		if entry.Account != "AccountsReceivable" {
			t.Errorf("expected Account 'AccountsReceivable', got %q", entry.Account)
		}
		if !entry.Debit.Equal(decimal.NewFromFloat(1000.00)) {
			t.Errorf("expected Debit 1000.00, got %s", entry.Debit.String())
		}
		if !entry.Credit.Equal(decimal.Zero) {
			t.Errorf("expected Credit 0, got %s", entry.Credit.String())
		}
	})

	t.Run("creates credit ledger entry", func(t *testing.T) {
		entry := &LedgerEntry{
			ID:        LedgerEntryID("ledger-002"),
			InvoiceID: InvoiceID("inv-001"),
			TenantID:  TenantID("tenant-1"),
			Account:   AccountType("Revenue"),
			Debit:     decimal.NewFromFloat(0),
			Credit:    decimal.NewFromFloat(1000.00),
			CreatedAt: time.Now().UTC(),
		}

		if !entry.Credit.Equal(decimal.NewFromFloat(1000.00)) {
			t.Errorf("expected Credit 1000.00, got %s", entry.Credit.String())
		}
		if !entry.Debit.Equal(decimal.Zero) {
			t.Errorf("expected Debit 0, got %s", entry.Debit.String())
		}
	})
}

func TestLedger_AccountTypes(t *testing.T) {
	t.Run("account type constants", func(t *testing.T) {
		accounts := []struct {
			name string
			typ  AccountType
		}{
			{"AccountsReceivable", AccountType("AccountsReceivable")},
			{"Revenue", AccountType("Revenue")},
		}

		for _, a := range accounts {
			t.Run(a.name, func(t *testing.T) {
				if a.typ != AccountType(a.name) {
					t.Errorf("expected account type %q", a.name)
				}
			})
		}
	})
}
