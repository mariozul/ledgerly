CREATE TABLE ledger_entries (
    id VARCHAR(36) PRIMARY KEY,
    invoice_id VARCHAR(36) NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    tenant_id VARCHAR(36) NOT NULL,
    account VARCHAR(255) NOT NULL,
    debit NUMERIC(18, 4) NOT NULL,
    credit NUMERIC(18, 4) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255) NOT NULL
);

CREATE INDEX idx_ledger_entries_invoice_id ON ledger_entries (invoice_id);
CREATE INDEX idx_ledger_entries_tenant_id ON ledger_entries (tenant_id);
