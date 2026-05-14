package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad_SuccessWithDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	clearOptional(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Environment != EnvDevelopment {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvDevelopment)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.LogLevel != LogLevelInfo {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelInfo)
	}
	if cfg.LogFormat != LogFormatJSON {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, LogFormatJSON)
	}
	if cfg.WorkerCount != 8 {
		t.Errorf("WorkerCount = %d, want 8", cfg.WorkerCount)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 30s", cfg.ShutdownTimeout)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearOptional(t)
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing DATABASE_URL, got nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should mention DATABASE_URL, got: %v", err)
	}
}

func TestLoad_OverrideAll(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://prod/db")
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("WORKER_COUNT", "32")
	t.Setenv("SHUTDOWN_TIMEOUT", "1m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	want := Config{
		Environment:     EnvProduction,
		HTTPPort:        9090,
		DatabaseURL:     "postgres://prod/db",
		LogLevel:        LogLevelWarn,
		LogFormat:       LogFormatText,
		WorkerCount:     32,
		ShutdownTimeout: time.Minute,
	}
	if *cfg != want {
		t.Errorf("Load() = %+v, want %+v", *cfg, want)
	}

	if !cfg.IsProduction() {
		t.Errorf("IsProduction() = false, want true")
	}
	if cfg.IsDevelopment() {
		t.Errorf("IsDevelopment() = true, want false")
	}
}

func TestValidate_InvalidValues(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string // substring expected in error
	}{
		{
			name: "port too low",
			cfg:  baseValid(func(c *Config) { c.HTTPPort = 0 }),
			want: "HTTP_PORT",
		},
		{
			name: "port too high",
			cfg:  baseValid(func(c *Config) { c.HTTPPort = 70000 }),
			want: "HTTP_PORT",
		},
		{
			name: "worker count zero",
			cfg:  baseValid(func(c *Config) { c.WorkerCount = 0 }),
			want: "WORKER_COUNT",
		},
		{
			name: "negative worker count",
			cfg:  baseValid(func(c *Config) { c.WorkerCount = -1 }),
			want: "WORKER_COUNT",
		},
		{
			name: "unknown log format",
			cfg:  baseValid(func(c *Config) { c.LogFormat = "xml" }),
			want: "LOG_FORMAT",
		},
		{
			name: "unknown log level",
			cfg:  baseValid(func(c *Config) { c.LogLevel = "fatal" }),
			want: "LOG_LEVEL",
		},
		{
			name: "unknown environment",
			cfg:  baseValid(func(c *Config) { c.Environment = "qa" }),
			want: "ENVIRONMENT",
		},
		{
			name: "zero shutdown timeout",
			cfg:  baseValid(func(c *Config) { c.ShutdownTimeout = 0 }),
			want: "SHUTDOWN_TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidate_AggregatesAllErrors(t *testing.T) {
	cfg := Config{
		Environment:     "bad",
		HTTPPort:        0,
		DatabaseURL:     "postgres://x",
		LogLevel:        "loud",
		LogFormat:       "binary",
		WorkerCount:     0,
		ShutdownTimeout: -1,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want aggregated error")
	}
	got := err.Error()
	for _, want := range []string{"HTTP_PORT", "WORKER_COUNT", "SHUTDOWN_TIMEOUT", "LOG_FORMAT", "LOG_LEVEL", "ENVIRONMENT"} {
		if !strings.Contains(got, want) {
			t.Errorf("error should contain %q, got: %s", want, got)
		}
	}
}

// baseValid returns a known-good Config, optionally mutated by f.
func baseValid(f func(*Config)) Config {
	cfg := Config{
		Environment:     EnvDevelopment,
		HTTPPort:        8080,
		DatabaseURL:     "postgres://localhost/test",
		LogLevel:        LogLevelInfo,
		LogFormat:       LogFormatJSON,
		WorkerCount:     4,
		ShutdownTimeout: 10 * time.Second,
	}
	if f != nil {
		f(&cfg)
	}
	return cfg
}

func clearOptional(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENVIRONMENT", "HTTP_PORT", "LOG_LEVEL", "LOG_FORMAT",
		"WORKER_COUNT", "SHUTDOWN_TIMEOUT",
	} {
		t.Setenv(k, "")
	}
}
