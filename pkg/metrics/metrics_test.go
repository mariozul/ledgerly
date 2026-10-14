package metrics

import (
	"strings"
	"testing"
)

// TC-OBS-03-METRICS-UNIT: Metrics are emitted with bounded cardinality labels (RULE-OBS-03)
func TestMetrics_LabelCardinality(t *testing.T) {
	tests := []struct {
		name       string
		metricFunc func() MetricCollector
		labels     map[string]string
		wantPanic  bool
	}{
		{
			name:       "HTTP request duration has bounded labels",
			metricFunc: func() MetricCollector { return NewHTTPDurationCollector() },
			labels:     map[string]string{"path": "/v1/invoices", "method": "POST", "status_code": "200"},
			wantPanic:  false,
		},
		{
			name:       "Invoice creation counter has bounded labels",
			metricFunc: func() MetricCollector { return NewInvoiceCreationCollector() },
			labels:     map[string]string{"status": "success", "currency": "USD"},
			wantPanic:  false,
		},
		{
			name:       "FX provider call duration has bounded labels",
			metricFunc: func() MetricCollector { return NewFXProviderCallCollector() },
			labels:     map[string]string{"provider_name": "exchangerate-api", "status": "success", "currency_pair": "EUR_USD"},
			wantPanic:  false,
		},
		{
			name:       "Outbox event counter has bounded labels",
			metricFunc: func() MetricCollector { return NewOutboxEventCollector() },
			labels:     map[string]string{"event_type": "invoice.issued", "status": "saved"},
			wantPanic:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tt.metricFunc()
			// The collector should not panic when recording metrics with bounded labels
			collector.Record(tt.labels, 1.0)
		})
	}
}

// TC-OBS-03-METRICS-UNIT: Verify that high-cardinality labels are rejected
func TestMetrics_RejectsHighCardinalityLabels(t *testing.T) {
	tests := []struct {
		name       string
		metricFunc func() MetricCollector
		labels     map[string]string
	}{
		{
			name:       "Rejects invoice_id as label for HTTP duration",
			metricFunc: func() MetricCollector { return NewHTTPDurationCollector() },
			labels:     map[string]string{"path": "/v1/invoices", "method": "POST", "status_code": "200", "invoice_id": "123e4567-e89b-12d3-a456-426614174000"},
		},
		{
			name:       "Rejects tenant_id as label for invoice creation",
			metricFunc: func() MetricCollector { return NewInvoiceCreationCollector() },
			labels:     map[string]string{"status": "success", "currency": "USD", "tenant_id": "tenant-123"},
		},
		{
			name:       "Rejects full error message as metric label",
			metricFunc: func() MetricCollector { return NewHTTPDurationCollector() },
			labels:     map[string]string{"path": "/v1/invoices", "method": "POST", "status_code": "500", "error": "internal server error: something went wrong"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tt.metricFunc()
			err := collector.Record(tt.labels, 1.0)
			if err == nil {
				// If the collector accepted the labels, that's a violation of RULE-OBS-03
				// Check if any label key is a high-cardinality value
				for k := range tt.labels {
					low := strings.ToLower(k)
					if low == "invoice_id" || low == "tenant_id" || low == "idempotency_key" || low == "error" {
						t.Errorf("RULE-OBS-03 Violation: label %q is a high-cardinality value that should be rejected", k)
					}
				}
			}
		})
	}
}

// Verify that the metrics are properly registered and can be collected
func TestMetrics_Registration(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterHTTPDuration()
	registry.RegisterInvoiceCreation()
	registry.RegisterFXProviderCall()
	registry.RegisterOutboxEvent()

	// Should be able to gather without error
	_, err := registry.Gather()
	if err != nil {
		t.Errorf("Failed to gather metrics: %v", err)
	}
}
