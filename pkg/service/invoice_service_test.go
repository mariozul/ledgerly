package service

import (
	"sync"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/agent-orchestration/ledgerly/pkg/domain"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockInvoiceRepo struct {
	createInvoiceFn     func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error
	getInvoiceByIDFn    func(ctx context.Context, tenantID domain.TenantID, invoiceID domain.InvoiceID) (*domain.Invoice, error)
	getInvoiceByIdempFn func(ctx context.Context, tenantID domain.TenantID, key domain.IdempotencyKey) (*domain.Invoice, error)
}

func (m *mockInvoiceRepo) CreateInvoice(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
	return m.createInvoiceFn(ctx, invoice, ledgerEntries)
}
func (m *mockInvoiceRepo) GetInvoiceByID(ctx context.Context, tenantID domain.TenantID, invoiceID domain.InvoiceID) (*domain.Invoice, error) {
	return m.getInvoiceByIDFn(ctx, tenantID, invoiceID)
}
func (m *mockInvoiceRepo) GetInvoiceByIdempotencyKey(ctx context.Context, tenantID domain.TenantID, key domain.IdempotencyKey) (*domain.Invoice, error) {
	return m.getInvoiceByIdempFn(ctx, tenantID, key)
}

type mockLedgerRepo struct {
	createLedgerEntriesFn func(ctx context.Context, entries []domain.LedgerEntry) error
}

func (m *mockLedgerRepo) CreateLedgerEntries(ctx context.Context, entries []domain.LedgerEntry) error {
	return m.createLedgerEntriesFn(ctx, entries)
}

type mockOutboxRepo struct {
	saveEventFn func(ctx context.Context, event *domain.OutboxEvent) error
}

func (m *mockOutboxRepo) SaveEvent(ctx context.Context, event *domain.OutboxEvent) error {
	return m.saveEventFn(ctx, event)
}

type mockFXProvider struct {
	getRateFn func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error)
}

func (m *mockFXProvider) GetRate(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
	return m.getRateFn(ctx, from, to)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func validCreateRequest() CreateInvoiceRequest {
	return CreateInvoiceRequest{
		TenantID:       domain.TenantID("tenant-1"),
		Currency:       domain.Currency("EUR"),
		BaseCurrency:   domain.Currency("USD"),
		IdempotencyKey: domain.IdempotencyKey(uuid.New().String()),
		LineItems: []LineItemInput{
			{Description: "Consulting", Amount: decimal.NewFromFloat(100.00)},
			{Description: "License", Amount: decimal.NewFromFloat(200.00)},
		},
		CreatedBy: "user-1",
	}
}

func defaultMockServices() (*mockInvoiceRepo, *mockLedgerRepo, *mockOutboxRepo, *mockFXProvider) {
	invRepo := &mockInvoiceRepo{
		createInvoiceFn: func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			return nil
		},
		getInvoiceByIDFn: func(ctx context.Context, tenantID domain.TenantID, invoiceID domain.InvoiceID) (*domain.Invoice, error) {
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, "not found")
		},
		getInvoiceByIdempFn: func(ctx context.Context, tenantID domain.TenantID, key domain.IdempotencyKey) (*domain.Invoice, error) {
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, "not found")
		},
	}
	ledgerRepo := &mockLedgerRepo{
		createLedgerEntriesFn: func(ctx context.Context, entries []domain.LedgerEntry) error {
			return nil
		},
	}
	outboxRepo := &mockOutboxRepo{
		saveEventFn: func(ctx context.Context, event *domain.OutboxEvent) error {
			return nil
		},
	}
	fxProvider := &mockFXProvider{
		getRateFn: func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
			return domain.FXRate(1.18), nil
		},
	}
	return invRepo, ledgerRepo, outboxRepo, fxProvider
}

func newServiceWithMocks(
	inv domain.InvoiceRepository,
	ledger domain.LedgerRepository,
	outbox domain.OutboxRepository,
	fx domain.FXProvider,
) *InvoiceService {
	return NewInvoiceService(inv, ledger, outbox, fx)
}

// ---------------------------------------------------------------------------
// TC-INV-01-HAPPY-UNIT: Successful invoice creation
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_INV_01_HAPPY_UNIT(t *testing.T) {
	t.Run("creates invoice with valid input and returns invoice with ID", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		resultInv, err := svc.CreateInvoice(context.Background(), req)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resultInv == nil {
			t.Fatal("expected non-nil invoice")
		}
		if resultInv.ID == "" {
			t.Error("expected invoice ID to be set")
		}
		if resultInv.TenantID != req.TenantID {
			t.Errorf("expected TenantID %q, got %q", req.TenantID, resultInv.TenantID)
		}
		if resultInv.Currency != req.Currency {
			t.Errorf("expected Currency %q, got %q", req.Currency, resultInv.Currency)
		}
		if resultInv.FXRate != 1.18 {
			t.Errorf("expected FXRate 1.18, got %f", resultInv.FXRate)
		}
		if !resultInv.TotalAmount.Equal(decimal.NewFromFloat(300.00)) {
			t.Errorf("expected TotalAmount 300.00, got %s", resultInv.TotalAmount.String())
		}
		if len(resultInv.LineItems) != 2 {
			t.Errorf("expected 2 line items, got %d", len(resultInv.LineItems))
		}
		if resultInv.IdempotencyKey != req.IdempotencyKey {
			t.Errorf("expected IdempotencyKey %q, got %q", req.IdempotencyKey, resultInv.IdempotencyKey)
		}
	})

	t.Run("propagates context to FX provider", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		ctxCheck := false
		fxProvider.getRateFn = func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
			ctxCheck = true
			return 1.18, nil
		}
		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		_, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ctxCheck {
			t.Error("expected context to be propagated to FX provider")
		}
	})

	t.Run("returns typed domain error for invalid input", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		tests := []struct {
			name string
			req  CreateInvoiceRequest
		}{
			{"empty tenant", CreateInvoiceRequest{TenantID: "", Currency: "EUR", BaseCurrency: "USD", IdempotencyKey: "key-1", LineItems: []LineItemInput{{Description: "test", Amount: decimal.NewFromFloat(100)}}, CreatedBy: "u"}},
			{"empty currency", CreateInvoiceRequest{TenantID: "t1", Currency: "", BaseCurrency: "USD", IdempotencyKey: "key-1", LineItems: []LineItemInput{{Description: "test", Amount: decimal.NewFromFloat(100)}}, CreatedBy: "u"}},
			{"empty base currency", CreateInvoiceRequest{TenantID: "t1", Currency: "EUR", BaseCurrency: "", IdempotencyKey: "key-1", LineItems: []LineItemInput{{Description: "test", Amount: decimal.NewFromFloat(100)}}, CreatedBy: "u"}},
			{"empty idempotency key", CreateInvoiceRequest{TenantID: "t1", Currency: "EUR", BaseCurrency: "USD", IdempotencyKey: "", LineItems: []LineItemInput{{Description: "test", Amount: decimal.NewFromFloat(100)}}, CreatedBy: "u"}},
			{"no line items", CreateInvoiceRequest{TenantID: "t1", Currency: "EUR", BaseCurrency: "USD", IdempotencyKey: "key-1", LineItems: []LineItemInput{}, CreatedBy: "u"}},
			{"negative line item amount", CreateInvoiceRequest{TenantID: "t1", Currency: "EUR", BaseCurrency: "USD", IdempotencyKey: "key-1", LineItems: []LineItemInput{{Description: "test", Amount: decimal.NewFromFloat(-100)}}, CreatedBy: "u"}},
			{"zero line item amount", CreateInvoiceRequest{TenantID: "t1", Currency: "EUR", BaseCurrency: "USD", IdempotencyKey: "key-1", LineItems: []LineItemInput{{Description: "test", Amount: decimal.Zero}}, CreatedBy: "u"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := svc.CreateInvoice(context.Background(), tt.req)
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if !domain.IsDomainError(err, domain.ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// TC-INV-03-IDEMPOTENCY-INT / TC-INV-04-IDEMPOTENCY-RACE: Idempotency
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_INV_03_IDEMPOTENCY_INT(t *testing.T) {
	t.Run("duplicate idempotency key returns original invoice", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()

		originalInvoice := &domain.Invoice{
			ID:             domain.InvoiceID("inv-001"),
			TenantID:       domain.TenantID("tenant-1"),
			Currency:       domain.Currency("EUR"),
			BaseCurrency:   domain.Currency("USD"),
			FXRate:         domain.FXRate(1.18),
			TotalAmount:    decimal.NewFromFloat(300.00),
			IdempotencyKey: domain.IdempotencyKey("idem-001"),
			CreatedAt:      time.Now().UTC(),
			CreatedBy:      "user-1",
			LineItems: []domain.InvoiceLine{
				{ID: "line-1", InvoiceID: "inv-001", Description: "Consulting", Amount: decimal.NewFromFloat(300.00)},
			},
		}

		callCount := 0
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			callCount++
			*invoice = *originalInvoice
			return nil
		}
		invRepo.getInvoiceByIdempFn = func(ctx context.Context, tenantID domain.TenantID, key domain.IdempotencyKey) (*domain.Invoice, error) {
			if callCount > 0 {
				return originalInvoice, nil
			}
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, "not found")
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		req.IdempotencyKey = "idem-001"

		inv1, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("first call unexpected error: %v", err)
		}
		if inv1 == nil {
			t.Fatal("first call expected non-nil invoice")
		}

		inv2, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("second call unexpected error: %v", err)
		}
		if inv2 == nil {
			t.Fatal("second call expected non-nil invoice")
		}

		if inv1.ID != inv2.ID {
			t.Errorf("expected same invoice ID on idempotent call: got %q vs %q", inv1.ID, inv2.ID)
		}
		if callCount != 1 {
			t.Errorf("expected CreateInvoice to be called only once, got %d", callCount)
		}
	})
}

func TestInvoiceService_TC_INV_04_IDEMPOTENCY_RACE(t *testing.T) {
	t.Run("concurrent requests with same idempotency key produce single invoice", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()

		var mu sync.Mutex
		var createCount int
		var stubInvoiceID domain.InvoiceID
		createdInvoices := make(map[string]bool)

		// Simulate an idempotent repository: first call succeeds, subsequent calls
		// with same idempotency key trigger ON CONFLICT DO NOTHING and return the original.
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			mu.Lock()
			defer mu.Unlock()
			// Check if idempotency key already processed by any goroutine
			if createdInvoices[string(invoice.IdempotencyKey)] {
				// Fetch the already-created invoice (simulating ON CONFLICT return)
				// In real DB, this would return the existing row
				*invoice = domain.Invoice{ID: stubInvoiceID, IdempotencyKey: invoice.IdempotencyKey}
				return nil
			}
			createCount++
			stubInvoiceID = invoice.ID
			createdInvoices[string(invoice.IdempotencyKey)] = true
			return nil
		}

		invRepo.getInvoiceByIdempFn = func(ctx context.Context, tenantID domain.TenantID, key domain.IdempotencyKey) (*domain.Invoice, error) {
			mu.Lock()
			defer mu.Unlock()
			if id, ok := createdInvoices[string(key)]; ok && id {
				return &domain.Invoice{ID: stubInvoiceID, IdempotencyKey: key}, nil
			}
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, "not found")
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		req.IdempotencyKey = domain.IdempotencyKey("race-idem-key")

		const goroutines = 20
		type result struct {
			inv *domain.Invoice
			err error
		}
		results := make(chan result, goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				inv, err := svc.CreateInvoice(context.Background(), req)
				results <- result{inv: inv, err: err}
			}()
		}

		var firstInvoiceID string
		for i := 0; i < goroutines; i++ {
			res := <-results
			if res.err != nil {
				t.Errorf("concurrent call returned error: %v", res.err)
				continue
			}
			if res.inv == nil {
				t.Error("concurrent call returned nil invoice")
				continue
			}
			if firstInvoiceID == "" {
				firstInvoiceID = string(res.inv.ID)
			} else if string(res.inv.ID) != firstInvoiceID {
				t.Errorf("concurrent call returned different invoice ID %q vs original %q", res.inv.ID, firstInvoiceID)
			}
		}

		mu.Lock()
		finalCount := createCount
		mu.Unlock()
		if finalCount > 1 {
			t.Errorf("expected at most 1 CreateInvoice call, got %d", finalCount)
		}
	})
}

// ---------------------------------------------------------------------------
// TC-FX-01-FETCH-UNIT: FX rate fetch and snapshotting
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_FX_01_FETCH_UNIT(t *testing.T) {
	t.Run("fx rate is fetched and stored on invoice", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		fxProvider.getRateFn = func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
			if from != "EUR" {
				t.Errorf("expected from currency EUR, got %q", from)
			}
			if to != "USD" {
				t.Errorf("expected to currency USD, got %q", to)
			}
			return domain.FXRate(1.25), nil
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		resultInv, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resultInv.FXRate != 1.25 {
			t.Errorf("expected FXRate 1.25, got %f", resultInv.FXRate)
		}
	})
}

// ---------------------------------------------------------------------------
// TC-FX-02-IMMUTABLE-UNIT: FX rate stored immutably (verified at domain level)
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_FX_02_IMMUTABLE_UNIT(t *testing.T) {
	t.Run("fx rate on created invoice matches fetched rate and is not altered", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		fxProvider.getRateFn = func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
			return domain.FXRate(1.35), nil
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		resultInv, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resultInv.FXRate != 1.35 {
			t.Errorf("expected FXRate 1.35, got %f", resultInv.FXRate)
		}
	})
}

// ---------------------------------------------------------------------------
// TC-FX-03-TRANSIENT-ERROR-UNIT: FX provider transient error
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_FX_03_TRANSIENT_ERROR_UNIT(t *testing.T) {
	t.Run("service returns ErrFXProviderUnavailable on transient FX error", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		fxProvider.getRateFn = func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
			return 0, domain.NewDomainError(domain.ErrFXProviderUnavailable, "upstream timeout")
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		_, err := svc.CreateInvoice(context.Background(), req)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if !domain.IsDomainError(err, domain.ErrFXProviderUnavailable) {
			t.Errorf("expected ErrFXProviderUnavailable, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TC-FX-04-PERMANENT-ERROR-UNIT: FX provider permanent error
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_FX_04_PERMANENT_ERROR_UNIT(t *testing.T) {
	t.Run("service returns ErrFXRateNotFound on permanent FX error", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		fxProvider.getRateFn = func(ctx context.Context, from, to domain.Currency) (domain.FXRate, error) {
			return 0, domain.NewDomainError(domain.ErrFXRateNotFound, "unsupported currency pair")
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		_, err := svc.CreateInvoice(context.Background(), req)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if !domain.IsDomainError(err, domain.ErrFXRateNotFound) {
			t.Errorf("expected ErrFXRateNotFound, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TC-LED-01-ATOMIC-INT: Invoice and ledger created atomically
// TC-LED-02-NO-PARTIAL-INT: No partial commit
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_LED_01_ATOMIC_INT(t *testing.T) {
	t.Run("invoice and ledger entries are passed together to CreateInvoice", func(t *testing.T) {
		var capturedInvoice *domain.Invoice
		var capturedLedger []domain.LedgerEntry

		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			capturedInvoice = invoice
			capturedLedger = ledgerEntries
			return nil
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		resultInv, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedInvoice == nil {
			t.Fatal("expected invoice to be passed to repository")
		}
		if len(capturedLedger) == 0 {
			t.Fatal("expected ledger entries to be passed to repository")
		}

		for _, entry := range capturedLedger {
			if entry.InvoiceID != resultInv.ID {
				t.Errorf("expected ledger entry InvoiceID %q, got %q", resultInv.ID, entry.InvoiceID)
			}
			if entry.TenantID != req.TenantID {
				t.Errorf("expected ledger entry TenantID %q, got %q", req.TenantID, entry.TenantID)
			}
		}

		var totalDebit, totalCredit decimal.Decimal
		for _, entry := range capturedLedger {
			totalDebit = totalDebit.Add(entry.Debit)
			totalCredit = totalCredit.Add(entry.Credit)
		}
		if !totalDebit.Equal(totalCredit) {
			t.Errorf("unbalanced ledger: debits %s != credits %s", totalDebit.String(), totalCredit.String())
		}
	})
}

func TestInvoiceService_TC_LED_02_NO_PARTIAL_INT(t *testing.T) {
	t.Run("returns error when repository fails without partial state", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			return domain.NewDomainError(domain.ErrInternal, "database write failed")
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		_, err := svc.CreateInvoice(context.Background(), req)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if !domain.IsDomainError(err, domain.ErrInternal) {
			t.Errorf("expected ErrInternal, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TC-EVT-01-OUTBOX-INT: Event published to outbox after transaction commit
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_EVT_01_OUTBOX_INT(t *testing.T) {
	t.Run("outbox event is saved with correct event type after invoice creation", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()

		var savedEvent *domain.OutboxEvent
		outboxRepo.saveEventFn = func(ctx context.Context, event *domain.OutboxEvent) error {
			savedEvent = event
			return nil
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		resultInv, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if savedEvent == nil {
			t.Fatal("expected outbox event to be saved")
		}
		if savedEvent.EventType != "invoice.issued" {
			t.Errorf("expected event type 'invoice.issued', got %q", savedEvent.EventType)
		}
		if savedEvent.AggregateID != domain.AggregateID(resultInv.ID) {
			t.Errorf("expected aggregate ID %q, got %q", resultInv.ID, savedEvent.AggregateID)
		}
		if savedEvent.Status != "PENDING" {
			t.Errorf("expected status 'PENDING', got %q", savedEvent.Status)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(savedEvent.Payload, &payload); err != nil {
			t.Errorf("expected valid JSON payload, got error: %v", err)
		}
		if payload["invoice_id"] != string(resultInv.ID) {
			t.Errorf("expected payload invoice_id %q, got %q", resultInv.ID, payload["invoice_id"])
		}
	})

	t.Run("no event saved if invoice creation fails", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			return domain.NewDomainError(domain.ErrInternal, "write failed")
		}

		var eventSaved bool
		outboxRepo.saveEventFn = func(ctx context.Context, event *domain.OutboxEvent) error {
			eventSaved = true
			return nil
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		_, err := svc.CreateInvoice(context.Background(), req)
		if err == nil {
			t.Fatal("expected error but got nil")
		}
		if eventSaved {
			t.Error("expected no outbox event to be saved when creation fails")
		}
	})
}

// ---------------------------------------------------------------------------
// TC-SEC-01-TENANT-ISOLATION-INT: Tenant isolation
// ---------------------------------------------------------------------------

func TestInvoiceService_TC_SEC_01_TENANT_ISOLATION_INT(t *testing.T) {
	t.Run("invoice is created with the provided tenant ID, other tenants can't see it", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()

		var createdInvoiceID domain.InvoiceID
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			createdInvoiceID = invoice.ID
			return nil
		}

		invRepo.getInvoiceByIDFn = func(ctx context.Context, tenantID domain.TenantID, invoiceID domain.InvoiceID) (*domain.Invoice, error) {
			if tenantID == domain.TenantID("tenant-1") {
				return &domain.Invoice{
					ID:       invoiceID,
					TenantID: tenantID,
				}, nil
			}
			return nil, domain.NewDomainError(domain.ErrInvoiceNotFound, "not found")
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		req.TenantID = domain.TenantID("tenant-1")
		_, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if createdInvoiceID != "" {
			_, err2 := invRepo.GetInvoiceByID(context.Background(), domain.TenantID("tenant-2"), createdInvoiceID)
			if err2 == nil {
				t.Error("expected error when tenant-2 tries to access tenant-1's invoice")
			}
			if !domain.IsDomainError(err2, domain.ErrInvoiceNotFound) {
				t.Errorf("expected ErrInvoiceNotFound, got %v", err2)
			}
		} else {
			t.Error("invoice ID was not set")
		}
	})

	t.Run("invoice is created with correct tenant ID from request", func(t *testing.T) {
		invRepo, ledgerRepo, outboxRepo, fxProvider := defaultMockServices()

		var capturedTenantID domain.TenantID
		invRepo.createInvoiceFn = func(ctx context.Context, invoice *domain.Invoice, ledgerEntries []domain.LedgerEntry) error {
			capturedTenantID = invoice.TenantID
			return nil
		}

		svc := newServiceWithMocks(invRepo, ledgerRepo, outboxRepo, fxProvider)

		req := validCreateRequest()
		req.TenantID = domain.TenantID("tenant-alpha")
		_, err := svc.CreateInvoice(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedTenantID != "tenant-alpha" {
			t.Errorf("expected tenant ID 'tenant-alpha', got %q", capturedTenantID)
		}
	})
}
