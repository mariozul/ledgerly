package fxprovider

import (
	"errors"
	"time"
)

// Config holds configuration for the HTTP FX provider client.
type Config struct {
	// BaseURL is the base URL of the external FX rate API.
	BaseURL string `yaml:"base_url" json:"base_url"`

	// APIKey is the authentication key for the external FX API (if required).
	APIKey string `yaml:"api_key" json:"api_key"`

	// Timeout is the maximum duration for a single HTTP request.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// RetryMaxAttempts is the maximum number of retry attempts for transient errors.
	RetryMaxAttempts int `yaml:"retry_max_attempts" json:"retry_max_attempts"`

	// RetryBaseInterval is the initial backoff duration between retries.
	RetryBaseInterval time.Duration `yaml:"retry_base_interval" json:"retry_base_interval"`

	// RetryMaxInterval caps the exponential backoff duration.
	RetryMaxInterval time.Duration `yaml:"retry_max_interval" json:"retry_max_interval"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:           10 * time.Second,
		RetryMaxAttempts:  3,
		RetryBaseInterval: 500 * time.Millisecond,
		RetryMaxInterval:  5 * time.Second,
	}
}

// Validate checks that the config has valid values.
func (c Config) Validate() error {
	if c.BaseURL == "" {
		return errors.New("base_url is required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.RetryMaxAttempts < 0 {
		return errors.New("retry_max_attempts must be >= 0")
	}
	if c.RetryBaseInterval <= 0 {
		return errors.New("retry_base_interval must be positive")
	}
	if c.RetryMaxInterval <= 0 {
		return errors.New("retry_max_interval must be positive")
	}
	return nil
}
