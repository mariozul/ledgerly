package handler

import (
	"time"

	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// Request DTOs
// ---------------------------------------------------------------------------

// CreateInvoiceRequest is the HTTP request body for creating an invoice.
type CreateInvoiceRequest struct {
	Currency       string              `json:"currency"`
	BaseCurrency   string              `json:"base_currency"`
	IdempotencyKey string              `json:"idempotency_key"`
	LineItems      []InvoiceLineItem   `json:"line_items"`
}

// InvoiceLineItem represents a single line item in the create request.
type InvoiceLineItem struct {
	Description string          `json:"description"`
	Amount      decimal.Decimal `json:"amount"`
}

// ---------------------------------------------------------------------------
// Response DTOs
// ---------------------------------------------------------------------------

// InvoiceResponse is the HTTP response for a created invoice.
type InvoiceResponse struct {
	ID             string                `json:"id"`
	TenantID       string                `json:"tenant_id"`
	Currency       string                `json:"currency"`
	BaseCurrency   string                `json:"base_currency"`
	FXRate         float64               `json:"fx_rate"`
	TotalAmount    decimal.Decimal       `json:"total_amount"`
	LineItems      []InvoiceLineResponse `json:"line_items"`
	IdempotencyKey string                `json:"idempotency_key"`
	CreatedAt      time.Time             `json:"created_at"`
}

// InvoiceLineResponse represents a line item in the response.
type InvoiceLineResponse struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Amount      decimal.Decimal `json:"amount"`
}

// ErrorResponse is the standard error response.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
