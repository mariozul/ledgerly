package domain

import "context"

// FXProvider defines the contract for fetching foreign exchange rates.
// Abstract interface - implementations reside in infrastructure layer (RULE-ARCH-02, FR-FX-01).
type FXProvider interface {
	// GetRate fetches the exchange rate from the source currency to the target currency.
	// Propagates ctx for cancellation and tracing (RULE-RES-01, RULE-OBS-02).
	// Returns:
	//   - FXRate on success
	//   - ErrFXProviderUnavailable for transient errors (FR-FX-02)
	//   - ErrFXRateNotFound if the currency pair is not supported (FR-FX-02)
	GetRate(ctx context.Context, from Currency, to Currency) (FXRate, error)
}
