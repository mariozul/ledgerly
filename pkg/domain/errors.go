package domain

import (
	"errors"
	"fmt"
)

// Domain error sentinels - RULE-ARCH-03: typed error taxonomy
var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrInvoiceNotFound       = errors.New("invoice not found")
	ErrAlreadyExists         = errors.New("invoice with idempotency key already exists")
	ErrFXProviderUnavailable = errors.New("FX provider unavailable")
	ErrFXRateNotFound        = errors.New("FX rate not found")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrForbidden             = errors.New("forbidden")
	ErrInternal              = errors.New("internal server error")
)

// DomainError wraps a sentinel with additional context.
type DomainError struct {
	Err     error
	Message string
}

func (e *DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Err.Error(), e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewDomainError creates a new DomainError.
func NewDomainError(err error, msg string) *DomainError {
	return &DomainError{Err: err, Message: msg}
}

// IsDomainError checks if a given error is a DomainError wrapping the target sentinel.
func IsDomainError(err, target error) bool {
	return errors.Is(err, target)
}
