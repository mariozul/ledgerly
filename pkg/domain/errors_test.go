package domain

import (
	"errors"
	"testing"
)

// TC-FX-04-PERMANENT-ERROR-UNIT (partial): Verify domain error sentinels exist and work.
func TestDomainErrors_Sentinels(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{"ErrInvalidInput", ErrInvalidInput, "invalid input"},
		{"ErrInvoiceNotFound", ErrInvoiceNotFound, "invoice not found"},
		{"ErrAlreadyExists", ErrAlreadyExists, "invoice with idempotency key already exists"},
		{"ErrFXProviderUnavailable", ErrFXProviderUnavailable, "FX provider unavailable"},
		{"ErrFXRateNotFound", ErrFXRateNotFound, "FX rate not found"},
		{"ErrUnauthorized", ErrUnauthorized, "unauthorized"},
		{"ErrForbidden", ErrForbidden, "forbidden"},
		{"ErrInternal", ErrInternal, "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.message {
				t.Errorf("expected error message %q, got %q", tt.message, tt.err.Error())
			}
		})
	}
}

func TestDomainErrors_Wrapping(t *testing.T) {
	t.Run("DomainError wraps sentinel correctly", func(t *testing.T) {
		original := ErrFXProviderUnavailable
		domainErr := NewDomainError(original, "provider timeout")

		if !errors.Is(domainErr, original) {
			t.Errorf("DomainError should wrap the sentinel error")
		}

		if !IsDomainError(domainErr, original) {
			t.Errorf("IsDomainError should recognize wrapped sentinel")
		}
	})

	t.Run("DomainError message format", func(t *testing.T) {
		domainErr := NewDomainError(ErrFXRateNotFound, "EUR/USD not found")
		expected := "FX rate not found: EUR/USD not found"
		if domainErr.Error() != expected {
			t.Errorf("expected %q, got %q", expected, domainErr.Error())
		}
	})

	t.Run("IsDomainError returns false for unrelated errors", func(t *testing.T) {
		unrelated := errors.New("something else")
		if IsDomainError(unrelated, ErrFXProviderUnavailable) {
			t.Errorf("IsDomainError should return false for unrelated errors")
		}
	})
}
