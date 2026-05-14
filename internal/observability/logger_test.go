package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLogger_JSONHasStandardFields(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(Options{
		Level:  "info",
		Format: FormatJSON,
		Output: &buf,
	})
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	logger.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw=%q", err, buf.String())
	}

	for _, field := range []string{"time", "level", "msg", "source"} {
		if _, ok := rec[field]; !ok {
			t.Errorf("missing standard field %q in record: %v", field, rec)
		}
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v, want %q", rec["msg"], "hello")
	}
	if rec["k"] != "v" {
		t.Errorf("custom attr k = %v, want %q", rec["k"], "v")
	}
}

func TestNewLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(Options{Level: "warn", Format: FormatJSON, Output: &buf})
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")
	logger.Error("e")

	got := buf.String()
	if strings.Contains(got, `"msg":"d"`) || strings.Contains(got, `"msg":"i"`) {
		t.Errorf("debug/info should have been filtered: %s", got)
	}
	if !strings.Contains(got, `"msg":"w"`) || !strings.Contains(got, `"msg":"e"`) {
		t.Errorf("warn/error should be emitted: %s", got)
	}
}

func TestNewLogger_UnknownLevel(t *testing.T) {
	_, err := NewLogger(Options{Level: "loud"})
	if err == nil {
		t.Fatal("NewLogger() expected error for unknown level")
	}
}

func TestNewLogger_UnknownFormat(t *testing.T) {
	_, err := NewLogger(Options{Level: "info", Format: "xml"})
	if err == nil {
		t.Fatal("NewLogger() expected error for unknown format")
	}
}

func TestNewLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(Options{Level: "info", Format: FormatText, Output: &buf})
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}
	logger.Info("hello")
	if !strings.Contains(buf.String(), "msg=hello") {
		t.Errorf("text output should contain msg=hello, got: %s", buf.String())
	}
}

func TestContextHelpers_PropagateAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(Options{Level: "info", Format: FormatJSON, Output: &buf})
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	ctx := WithCorrelationID(context.Background(), "corr-123")
	ctx = WithJobID(ctx, "job-456")
	ctx = WithWorkerID(ctx, "worker-7")

	logger.InfoContext(ctx, "processing")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("bad JSON: %v\nraw=%q", err, buf.String())
	}

	if rec["correlation_id"] != "corr-123" {
		t.Errorf("correlation_id = %v, want corr-123", rec["correlation_id"])
	}
	if rec["job_id"] != "job-456" {
		t.Errorf("job_id = %v, want job-456", rec["job_id"])
	}
	if rec["worker_id"] != "worker-7" {
		t.Errorf("worker_id = %v, want worker-7", rec["worker_id"])
	}
}

func TestContextHelpers_AbsentValuesNotEmitted(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(Options{Level: "info", Format: FormatJSON, Output: &buf})
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	logger.InfoContext(context.Background(), "no-ctx")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	for _, k := range []string{"correlation_id", "job_id", "worker_id"} {
		if _, ok := rec[k]; ok {
			t.Errorf("did not expect %q in record without context value: %v", k, rec)
		}
	}
}

func TestCorrelationID_Getter(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "abc")
	if got := CorrelationID(ctx); got != "abc" {
		t.Errorf("CorrelationID() = %q, want %q", got, "abc")
	}
	if got := CorrelationID(context.Background()); got != "" {
		t.Errorf("CorrelationID(empty) = %q, want empty", got)
	}
}

func TestWithAttrs_PreservesContextHandling(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(Options{Level: "info", Format: FormatJSON, Output: &buf})
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	// .With(...) creates a derived logger that still propagates ctx attrs.
	derived := logger.With("component", "scheduler")
	ctx := WithCorrelationID(context.Background(), "x")
	derived.InfoContext(ctx, "started")

	got := buf.String()
	if !strings.Contains(got, `"component":"scheduler"`) {
		t.Errorf("missing 'component' attribute: %s", got)
	}
	if !strings.Contains(got, `"correlation_id":"x"`) {
		t.Errorf("missing 'correlation_id' from context: %s", got)
	}
}
