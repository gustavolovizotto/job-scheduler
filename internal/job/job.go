// Package job is the domain core: it owns the Job entity, the Status state
// machine and the construction/validation rules.
//
// This package is the innermost layer of the system. By design it depends
// only on the standard library and a uuid library — never on storage, HTTP
// or any infrastructure concern. Anything in api/, storage/, worker/, etc.
// imports job, not the other way around.
package job

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Type identifies which handler the worker should invoke for a given job.
// Examples: "send_email", "thumbnail_generate", "report_compile".
type Type string

// Default bounds for new jobs.
const (
	DefaultMaxAttempts = 3
	MinPriority        = -100
	MaxPriority        = 100
)

// Job is the central domain entity. It represents one unit of background
// work scheduled for execution.
//
// Field invariants enforced by the constructor:
//
//   - ID is always a fresh UUIDv4.
//   - Type is non-empty.
//   - Payload is a valid JSON document (defaults to "{}").
//   - Priority is in [MinPriority, MaxPriority].
//   - MaxAttempts >= 1.
//   - Status is either StatusPending (RunAt is now/past) or StatusScheduled
//     (RunAt is in the future).
type Job struct {
	ID             uuid.UUID
	Type           Type
	Payload        json.RawMessage
	Status         Status
	Priority       int
	Attempts       int
	MaxAttempts    int
	RunAt          time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	IdempotencyKey *string
	LastError      *string
}

// NewJobParams groups the inputs to NewJob.
//
// All fields are optional except Type. Zero values get sensible defaults so
// callers can write `job.NewJob(job.NewJobParams{Type: "send_email"})` for
// the simple case.
type NewJobParams struct {
	Type           Type
	Payload        json.RawMessage // default: {}
	Priority       int             // default: 0
	MaxAttempts    int             // default: DefaultMaxAttempts (3)
	RunAt          time.Time       // default: now() — runs immediately
	IdempotencyKey *string         // default: nil
}

// NewJob constructs and validates a Job. It is the only sanctioned way to
// create a Job — direct struct literals bypass the invariants this function
// enforces.
//
// The returned Job has Status set to either StatusPending (RunAt <= now)
// or StatusScheduled (RunAt > now).
func NewJob(p NewJobParams) (*Job, error) {
	if p.Type == "" {
		return nil, ErrInvalidType
	}
	if p.Priority < MinPriority || p.Priority > MaxPriority {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidPriority, p.Priority)
	}
	if p.MaxAttempts == 0 {
		p.MaxAttempts = DefaultMaxAttempts
	} else if p.MaxAttempts < 1 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidMaxAttempts, p.MaxAttempts)
	}

	payload := p.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	} else if !json.Valid(payload) {
		return nil, ErrInvalidPayload
	}

	now := time.Now().UTC()
	runAt := p.RunAt
	if runAt.IsZero() {
		runAt = now
	} else {
		runAt = runAt.UTC()
	}

	status := StatusPending
	if runAt.After(now) {
		status = StatusScheduled
	}

	return &Job{
		ID:             uuid.New(),
		Type:           p.Type,
		Payload:        payload,
		Status:         status,
		Priority:       p.Priority,
		Attempts:       0,
		MaxAttempts:    p.MaxAttempts,
		RunAt:          runAt,
		CreatedAt:      now,
		UpdatedAt:      now,
		IdempotencyKey: p.IdempotencyKey,
	}, nil
}

// CanTransitionTo delegates to the Status state machine.
func (j *Job) CanTransitionTo(target Status) bool {
	return j.Status.CanTransitionTo(target)
}

// TransitionTo moves the Job to target if the state machine allows it. The
// UpdatedAt timestamp is bumped on success. StartedAt/CompletedAt are set
// when transitioning into running and into a terminal state respectively.
//
// Returns ErrInvalidTransition (wrapped) if the move is not permitted.
func (j *Job) TransitionTo(target Status) error {
	if !j.Status.CanTransitionTo(target) {
		return fmt.Errorf("from %q to %q: %w", j.Status, target, ErrInvalidTransition)
	}

	now := time.Now().UTC()
	j.Status = target
	j.UpdatedAt = now

	switch target {
	case StatusRunning:
		if j.StartedAt == nil {
			t := now
			j.StartedAt = &t
		}
	case StatusCompleted, StatusDeadLetter, StatusCancelled:
		t := now
		j.CompletedAt = &t
	}
	return nil
}

// IsRetryable reports whether this job still has retries left. A failed job
// with Attempts < MaxAttempts is retryable; one that has exhausted its
// budget should move to dead_letter.
func (j *Job) IsRetryable() bool {
	return j.Attempts < j.MaxAttempts
}

// RecordFailure marks a failed attempt: bumps Attempts, stores the error
// message, and bumps UpdatedAt. Does NOT change Status — the caller decides
// between StatusFailed (retry) and StatusDeadLetter (give up) and then calls
// TransitionTo accordingly.
func (j *Job) RecordFailure(errMsg string) {
	j.Attempts++
	j.LastError = &errMsg
	j.UpdatedAt = time.Now().UTC()
}
