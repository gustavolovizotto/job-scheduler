package job

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewJob_Defaults(t *testing.T) {
	j, err := NewJob(NewJobParams{Type: "send_email"})
	if err != nil {
		t.Fatalf("NewJob() unexpected error: %v", err)
	}

	if j.ID.String() == "" {
		t.Error("ID should be set")
	}
	if j.Type != "send_email" {
		t.Errorf("Type = %q, want send_email", j.Type)
	}
	if string(j.Payload) != "{}" {
		t.Errorf("Payload = %q, want {}", string(j.Payload))
	}
	if j.Status != StatusPending {
		t.Errorf("Status = %q, want pending", j.Status)
	}
	if j.Priority != 0 {
		t.Errorf("Priority = %d, want 0", j.Priority)
	}
	if j.MaxAttempts != DefaultMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", j.MaxAttempts, DefaultMaxAttempts)
	}
	if j.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", j.Attempts)
	}
	if j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() || j.RunAt.IsZero() {
		t.Errorf("timestamps should not be zero: %+v", j)
	}
	if j.IdempotencyKey != nil {
		t.Errorf("IdempotencyKey should be nil by default, got %v", j.IdempotencyKey)
	}
}

func TestNewJob_FutureRunAtBecomesScheduled(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	j, err := NewJob(NewJobParams{Type: "ping", RunAt: future})
	if err != nil {
		t.Fatalf("NewJob() error: %v", err)
	}
	if j.Status != StatusScheduled {
		t.Errorf("Status = %q, want scheduled (future run_at)", j.Status)
	}
}

func TestNewJob_PastRunAtBecomesPending(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	j, err := NewJob(NewJobParams{Type: "ping", RunAt: past})
	if err != nil {
		t.Fatalf("NewJob() error: %v", err)
	}
	if j.Status != StatusPending {
		t.Errorf("Status = %q, want pending", j.Status)
	}
}

func TestNewJob_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		params  NewJobParams
		wantErr error
	}{
		{
			name:    "missing type",
			params:  NewJobParams{},
			wantErr: ErrInvalidType,
		},
		{
			name:    "priority too low",
			params:  NewJobParams{Type: "t", Priority: -101},
			wantErr: ErrInvalidPriority,
		},
		{
			name:    "priority too high",
			params:  NewJobParams{Type: "t", Priority: 101},
			wantErr: ErrInvalidPriority,
		},
		{
			name:    "negative max attempts",
			params:  NewJobParams{Type: "t", MaxAttempts: -1},
			wantErr: ErrInvalidMaxAttempts,
		},
		{
			name:    "malformed payload",
			params:  NewJobParams{Type: "t", Payload: json.RawMessage(`{not json`)},
			wantErr: ErrInvalidPayload,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewJob(tt.params)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewJob() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewJob_PreservesValidJSONPayload(t *testing.T) {
	payload := json.RawMessage(`{"to":"user@example.com","subject":"hi"}`)
	j, err := NewJob(NewJobParams{Type: "send_email", Payload: payload})
	if err != nil {
		t.Fatalf("NewJob() error: %v", err)
	}
	if string(j.Payload) != string(payload) {
		t.Errorf("payload mutated: got %q, want %q", j.Payload, payload)
	}
}

func TestJob_TransitionTo_ValidUpdatesTimestamps(t *testing.T) {
	j := newPendingJob(t)
	before := j.UpdatedAt

	// Sleep to ensure UpdatedAt strictly advances.
	time.Sleep(time.Millisecond)

	if err := j.TransitionTo(StatusRunning); err != nil {
		t.Fatalf("TransitionTo(running) error: %v", err)
	}
	if j.Status != StatusRunning {
		t.Errorf("Status = %q, want running", j.Status)
	}
	if !j.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt should advance; before=%v after=%v", before, j.UpdatedAt)
	}
	if j.StartedAt == nil {
		t.Error("StartedAt should be set when transitioning to running")
	}
}

func TestJob_TransitionTo_CompletedSetsCompletedAt(t *testing.T) {
	j := newPendingJob(t)
	mustTransition(t, j, StatusRunning)
	mustTransition(t, j, StatusCompleted)

	if j.CompletedAt == nil {
		t.Error("CompletedAt should be set after reaching completed")
	}
}

func TestJob_TransitionTo_Invalid(t *testing.T) {
	j := newPendingJob(t)
	// pending → completed is not allowed (must go through running).
	err := j.TransitionTo(StatusCompleted)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
	if j.Status != StatusPending {
		t.Errorf("Status should be unchanged on invalid transition, got %q", j.Status)
	}
}

func TestJob_TransitionTo_TerminalIsLocked(t *testing.T) {
	j := newPendingJob(t)
	mustTransition(t, j, StatusRunning)
	mustTransition(t, j, StatusCompleted)

	for _, to := range AllStatuses() {
		if err := j.TransitionTo(to); err == nil {
			t.Errorf("terminal completed should not transition to %q", to)
		}
	}
}

func TestJob_RetryFlow(t *testing.T) {
	j := newPendingJob(t)
	j.MaxAttempts = 2

	mustTransition(t, j, StatusRunning)
	j.RecordFailure("boom")
	mustTransition(t, j, StatusFailed)

	if j.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", j.Attempts)
	}
	if !j.IsRetryable() {
		t.Error("should still be retryable")
	}
	if j.LastError == nil || *j.LastError != "boom" {
		t.Errorf("LastError = %v, want \"boom\"", j.LastError)
	}

	// retry
	mustTransition(t, j, StatusPending)
	mustTransition(t, j, StatusRunning)
	j.RecordFailure("again")
	if j.IsRetryable() {
		t.Error("should NOT be retryable after second failure (max=2)")
	}
	mustTransition(t, j, StatusDeadLetter)
	if j.CompletedAt == nil {
		t.Error("CompletedAt should be set on terminal dead_letter")
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

func newPendingJob(t *testing.T) *Job {
	t.Helper()
	j, err := NewJob(NewJobParams{Type: "test"})
	if err != nil {
		t.Fatalf("setup: NewJob: %v", err)
	}
	return j
}

func mustTransition(t *testing.T, j *Job, target Status) {
	t.Helper()
	if err := j.TransitionTo(target); err != nil {
		t.Fatalf("TransitionTo(%q): %v", target, err)
	}
}
