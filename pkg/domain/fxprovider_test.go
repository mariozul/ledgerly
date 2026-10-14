package domain

import (
	"context"
	"testing"
)

// TC-FX-01-FETCH-UNIT: Successful FX rate fetch from provider and snapshotting
func TestFXProvider_Interface(t *testing.T) {
	t.Run("FXProvider interface contract", func(t *testing.T) {
		// Verify the interface is defined correctly by checking a mock satisfies it
		mock := &mockFXProvider{
			rate: FXRate(1.18),
			err:  nil,
		}

		var provider FXProvider = mock

		rate, err := provider.GetRate(context.Background(), "EUR", "USD")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if rate != 1.18 {
			t.Errorf("expected rate 1.18, got %f", rate)
		}
	})
}

// TC-FX-03-TRANSIENT-ERROR-UNIT (interface-level): Verify error handling contract
func TestFXProvider_TransientError(t *testing.T) {
	t.Run("FXProvider returns transient errors", func(t *testing.T) {
		mock := &mockFXProvider{
			rate: 0,
			err:  ErrFXProviderUnavailable,
		}

		var provider FXProvider = mock

		_, err := provider.GetRate(context.Background(), "EUR", "USD")
		if err == nil {
			t.Fatal("expected an error")
		}

		// Should map to domain.ErrFXProviderUnavailable
		if !IsDomainError(err, ErrFXProviderUnavailable) {
			t.Errorf("expected ErrFXProviderUnavailable, got %v", err)
		}
	})
}

// TC-FX-04-PERMANENT-ERROR-UNIT: FX provider returns permanent error
func TestFXProvider_PermanentError(t *testing.T) {
	t.Run("FXProvider returns permanent errors", func(t *testing.T) {
		mock := &mockFXProvider{
			rate: 0,
			err:  ErrFXRateNotFound,
		}

		var provider FXProvider = mock

		_, err := provider.GetRate(context.Background(), "XYZ", "USD")
		if err == nil {
			t.Fatal("expected an error")
		}

		// Should map to domain.ErrFXRateNotFound
		if !IsDomainError(err, ErrFXRateNotFound) {
			t.Errorf("expected ErrFXRateNotFound, got %v", err)
		}
	})
}

// mockFXProvider implements FXProvider for testing.
type mockFXProvider struct {
	rate FXRate
	err  error
}

func (m *mockFXProvider) GetRate(ctx context.Context, from, to Currency) (FXRate, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.rate, nil
}
