package job

import "errors"

// Domain errors. Callers in upper layers (API, worker) inspect these with
// errors.Is to map them to HTTP status codes or operational decisions.
var (
	// ErrInvalidType is returned when a job is constructed with an empty type.
	ErrInvalidType = errors.New("job: type is required")

	// ErrInvalidPayload is returned when the payload is not valid JSON.
	ErrInvalidPayload = errors.New("job: payload must be valid JSON")

	// ErrInvalidPriority is returned when priority is outside [-100, 100].
	ErrInvalidPriority = errors.New("job: priority must be in [-100, 100]")

	// ErrInvalidMaxAttempts is returned when MaxAttempts < 1.
	ErrInvalidMaxAttempts = errors.New("job: max_attempts must be >= 1")

	// ErrInvalidTransition is returned by TransitionTo when the requested
	// status change is not allowed by the state machine.
	ErrInvalidTransition = errors.New("job: invalid status transition")
)
