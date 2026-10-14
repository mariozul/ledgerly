CREATE TABLE invoices (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    base_currency VARCHAR(3) NOT NULL,
    fx_rate NUMERIC(10, 6) NOT NULL,
    total_amount NUMERIC(18, 4) NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(255),
    deleted_at TIMESTAMPTZ,
    deleted_by VARCHAR(255),
    UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX idx_invoices_tenant_id ON invoices (tenant_id);
