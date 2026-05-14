// Package observability holds shared infrastructure for logs, metrics and
// tracing. This file defines the structured logger built on top of log/slog.
//
// The logger:
//
//   - emits JSON in production, human-readable text in development;
//   - automatically promotes context values (correlation_id, job_id,
//     worker_id) to log attributes so that a single search filter pulls every
//     log line related to one request or one job;
//   - exposes WithCorrelationID/WithJobID/WithWorkerID helpers that callers
//     use to attach the relevant identifier without touching the logger.
package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Format selects between JSON and text log output.
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Options configures the logger created by NewLogger.
type Options struct {
	Level  string    // "debug" | "info" | "warn" | "error"
	Format Format    // "json" (default) | "text"
	Output io.Writer // defaults to os.Stdout
}

// NewLogger constructs a slog.Logger configured per opts. It returns an error
// if the level or format is unknown so misconfiguration fails fast at boot.
//
// Source location (file:line) is always included — operators need it to
// triage production issues.
func NewLogger(opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	var inner slog.Handler
	switch opts.Format {
	case FormatJSON, "":
		inner = slog.NewJSONHandler(out, handlerOpts)
	case FormatText:
		inner = slog.NewTextHandler(out, handlerOpts)
	default:
		return nil, fmt.Errorf("observability: unknown log format %q", opts.Format)
	}

	return slog.New(&contextHandler{inner: inner}), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("observability: unknown log level %q", s)
	}
}

// ─── Context-aware handler ───────────────────────────────────────────────────

type ctxKey int

const (
	ctxKeyCorrelationID ctxKey = iota
	ctxKeyJobID
	ctxKeyWorkerID
)

// WithCorrelationID returns a context with correlation_id attached. Every
// subsequent log line written through a logger built by NewLogger will carry
// the id automatically.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelationID, id)
}

// CorrelationID retrieves the correlation id from ctx, or "" if absent.
func CorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyCorrelationID).(string)
	return v
}

// WithJobID attaches a job id to the context.
func WithJobID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyJobID, id)
}

// WithWorkerID attaches a worker id to the context.
func WithWorkerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyWorkerID, id)
}

// contextHandler is a slog.Handler decorator that copies well-known context
// values onto each record before delegating.
type contextHandler struct {
	inner slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v, ok := ctx.Value(ctxKeyCorrelationID).(string); ok && v != "" {
		r.AddAttrs(slog.String("correlation_id", v))
	}
	if v, ok := ctx.Value(ctxKeyJobID).(string); ok && v != "" {
		r.AddAttrs(slog.String("job_id", v))
	}
	if v, ok := ctx.Value(ctxKeyWorkerID).(string); ok && v != "" {
		r.AddAttrs(slog.String("worker_id", v))
	}
	return h.inner.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{inner: h.inner.WithGroup(name)}
}
