// Package config loads, validates and exposes the application configuration.
//
// Configuration is read from environment variables (with optional .env support
// in development). All values are validated at startup — the app must fail
// fast with a clear error message rather than start with a broken config.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Environment is a typed identifier for the runtime environment.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// LogFormat enumerates the supported log output formats.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// LogLevel enumerates the supported log levels.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Config holds the fully-validated runtime configuration.
//
// Fields are populated from environment variables. Required fields without a
// default cause Load() to fail. See .env.example for the full list with
// documentation.
type Config struct {
	// Environment toggles dev-friendly behavior (pretty logs, .env loading).
	Environment Environment `env:"ENVIRONMENT" envDefault:"development"`

	// HTTPPort is the port the API server listens on.
	HTTPPort int `env:"HTTP_PORT" envDefault:"8080"`

	// DatabaseURL is the Postgres connection string. Required — no default.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// LogLevel controls the minimum log level emitted.
	LogLevel LogLevel `env:"LOG_LEVEL" envDefault:"info"`

	// LogFormat selects between structured JSON and human-readable text.
	LogFormat LogFormat `env:"LOG_FORMAT" envDefault:"json"`

	// WorkerCount is the number of concurrent worker goroutines.
	WorkerCount int `env:"WORKER_COUNT" envDefault:"8"`

	// ShutdownTimeout is the grace period for in-flight requests/jobs during
	// graceful shutdown.
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// Load reads configuration from the environment, optionally augmenting it
// from a local .env file (development only), validates it, and returns it.
//
// The .env file is best-effort: a missing file is not an error. This keeps
// production deploys (where config comes from the orchestrator) clean.
func Load() (*Config, error) {
	// Best-effort .env load — missing file is fine.
	_ = godotenv.Load()

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("config: parsing env: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: invalid: %w", err)
	}

	return &cfg, nil
}

// Validate enforces semantic rules that struct tags cannot express.
func (c *Config) Validate() error {
	var errs []error

	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		errs = append(errs, fmt.Errorf("HTTP_PORT must be 1..65535, got %d", c.HTTPPort))
	}
	if c.WorkerCount < 1 {
		errs = append(errs, fmt.Errorf("WORKER_COUNT must be >= 1, got %d", c.WorkerCount))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be > 0, got %s", c.ShutdownTimeout))
	}

	switch c.LogFormat {
	case LogFormatJSON, LogFormatText:
	default:
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be %q or %q, got %q",
			LogFormatJSON, LogFormatText, c.LogFormat))
	}

	switch c.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug|info|warn|error, got %q", c.LogLevel))
	}

	switch c.Environment {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("ENVIRONMENT must be development|staging|production, got %q", c.Environment))
	}

	return errors.Join(errs...)
}

// IsProduction reports whether the app is running in the production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvProduction
}

// IsDevelopment reports whether the app is running in development.
func (c *Config) IsDevelopment() bool {
	return c.Environment == EnvDevelopment
}
