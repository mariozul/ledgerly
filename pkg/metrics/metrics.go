package metrics

import (
	"fmt"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// MetricCollector defines the contract for recording metric observations.
// Implementations must enforce bounded cardinality on labels (RULE-OBS-03).
type MetricCollector interface {
	Record(labels map[string]string, value float64) error
}

// MetricRegistry holds references to all metric collectors.
type MetricRegistry struct {
	mu sync.RWMutex
	// In a production system, this would use Prometheus histograms/counters.
	// For this implementation, we use simple in-memory stores that validate labels.
	collectors map[string]MetricCollector
}

// NewRegistry creates a new empty metric registry.
func NewRegistry() *MetricRegistry {
	return &MetricRegistry{
		collectors: make(map[string]MetricCollector),
	}
}

// RegisterHTTPDuration registers the HTTP request duration histogram.
func (r *MetricRegistry) RegisterHTTPDuration() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors["http_request_duration_seconds"] = NewHTTPDurationCollector()
}

// RegisterInvoiceCreation registers the invoice creation counter.
func (r *MetricRegistry) RegisterInvoiceCreation() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors["app_invoice_creation_total"] = NewInvoiceCreationCollector()
}

// RegisterFXProviderCall registers the FX provider call duration histogram.
func (r *MetricRegistry) RegisterFXProviderCall() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors["app_fx_provider_call_duration_seconds"] = NewFXProviderCallCollector()
}

// RegisterOutboxEvent registers the outbox event counter.
func (r *MetricRegistry) RegisterOutboxEvent() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors["app_outbox_event_total"] = NewOutboxEventCollector()
}

// Gather returns a snapshot of all registered metrics.
// In production, this would expose Prometheus output.
func (r *MetricRegistry) Gather() (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]interface{})
	for name := range r.collectors {
		result[name] = "registered"
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Allowed label sets per metric - enforcing bounded cardinality (RULE-OBS-03)
// These maps define which label keys are permitted for each metric type.
// HIGH-CARDINALITY VALUES (invoice_id, tenant_id, idempotency_key, full errors)
// are STRICTLY PROHIBITED as metric labels.
// ---------------------------------------------------------------------------

var allowedLabelsHTTPDuration = map[string]bool{
	"path":        true,
	"method":      true,
	"status_code": true,
}

var allowedLabelsInvoiceCreation = map[string]bool{
	"status":   true,
	"currency": true,
}

var allowedLabelsFXProviderCall = map[string]bool{
	"provider_name": true,
	"status":        true,
	"currency_pair": true,
}

var allowedLabelsOutboxEvent = map[string]bool{
	"event_type": true,
	"status":     true,
}

// blockedLabelKeys are high-cardinality keys that must never appear as metric labels (RULE-OBS-03).
var blockedLabelKeys = map[string]bool{
	"invoice_id":      true,
	"tenant_id":       true,
	"idempotency_key": true,
	"user_id":         true,
	"error":           true,
	"trace_id":        true,
	"span_id":         true,
}

// simpleCollector is a basic in-memory metric collector with label validation.
type simpleCollector struct {
	name          string
	allowedLabels map[string]bool
}

func (c *simpleCollector) Record(labels map[string]string, value float64) error {
	for k := range labels {
		// Check for blocked high-cardinality keys first (RULE-OBS-03)
		low := strings.ToLower(k)
		if blockedLabelKeys[low] {
			return fmt.Errorf("RULE-OBS-03 violation: label %q is a high-cardinality value that is not allowed as a metric label", k)
		}
		// Check for allowed labels
		if !c.allowedLabels[k] {
			return fmt.Errorf("RULE-OBS-03 violation: label %q is not in the allowed set for metric %s", k, c.name)
		}
	}
	// In production this would record to Prometheus
	return nil
}

// NewHTTPDurationCollector creates a collector for http_request_duration_seconds.
// Labels: path, method, status_code (RULE-OBS-03)
func NewHTTPDurationCollector() MetricCollector {
	return &simpleCollector{
		name:          "http_request_duration_seconds",
		allowedLabels: allowedLabelsHTTPDuration,
	}
}

// NewInvoiceCreationCollector creates a collector for app_invoice_creation_total.
// Labels: status, currency (RULE-OBS-03)
func NewInvoiceCreationCollector() MetricCollector {
	return &simpleCollector{
		name:          "app_invoice_creation_total",
		allowedLabels: allowedLabelsInvoiceCreation,
	}
}

// NewFXProviderCallCollector creates a collector for app_fx_provider_call_duration_seconds.
// Labels: provider_name, status, currency_pair (RULE-OBS-03)
func NewFXProviderCallCollector() MetricCollector {
	return &simpleCollector{
		name:          "app_fx_provider_call_duration_seconds",
		allowedLabels: allowedLabelsFXProviderCall,
	}
}

// NewOutboxEventCollector creates a collector for app_outbox_event_total.
// Labels: event_type, status (RULE-OBS-03)
func NewOutboxEventCollector() MetricCollector {
	return &simpleCollector{
		name:          "app_outbox_event_total",
		allowedLabels: allowedLabelsOutboxEvent,
	}
}
