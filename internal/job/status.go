package job

// Status is the lifecycle state of a Job.
//
// Transitions are validated by the state machine defined in this file — see
// CanTransitionTo and the validTransitions table.
//
// The full transition graph:
//
//	scheduled ─▶ pending  (run_at reached)
//	scheduled ─▶ cancelled
//	pending   ─▶ running  (worker pickup)
//	pending   ─▶ cancelled
//	running   ─▶ completed     (success — terminal)
//	running   ─▶ failed        (transient error)
//	running   ─▶ dead_letter   (permanent error or out of retries)
//	failed    ─▶ pending       (retry available)
//	failed    ─▶ dead_letter   (retries exhausted)
//
// Terminal statuses (completed, dead_letter, cancelled) have no outgoing
// transitions.
type Status string

const (
	// StatusScheduled — the job is waiting for its RunAt timestamp.
	StatusScheduled Status = "scheduled"
	// StatusPending — the job is ready to be picked up by a worker.
	StatusPending Status = "pending"
	// StatusRunning — a worker holds a lease on this job and is executing it.
	StatusRunning Status = "running"
	// StatusCompleted — handler returned nil. Terminal.
	StatusCompleted Status = "completed"
	// StatusFailed — handler returned a transient error; eligible for retry.
	StatusFailed Status = "failed"
	// StatusDeadLetter — gave up (permanent error or out of retries). Terminal.
	StatusDeadLetter Status = "dead_letter"
	// StatusCancelled — explicitly cancelled by the user. Terminal.
	StatusCancelled Status = "cancelled"
)

// AllStatuses returns every defined Status. Useful for tests and API docs.
func AllStatuses() []Status {
	return []Status{
		StatusScheduled,
		StatusPending,
		StatusRunning,
		StatusCompleted,
		StatusFailed,
		StatusDeadLetter,
		StatusCancelled,
	}
}

// IsValid reports whether s is one of the defined statuses.
func (s Status) IsValid() bool {
	switch s {
	case StatusScheduled, StatusPending, StatusRunning,
		StatusCompleted, StatusFailed, StatusDeadLetter, StatusCancelled:
		return true
	}
	return false
}

// IsTerminal reports whether s is an end state (no further transitions).
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusDeadLetter, StatusCancelled:
		return true
	}
	return false
}

// validTransitions encodes the state machine. Keys are source statuses;
// the inner map's keys are the legal destinations.
//
// Terminal statuses are present with empty inner maps so that IsValid()
// returns true but no transition is permitted.
var validTransitions = map[Status]map[Status]struct{}{
	StatusScheduled: {
		StatusPending:   {},
		StatusCancelled: {},
	},
	StatusPending: {
		StatusRunning:   {},
		StatusCancelled: {},
	},
	StatusRunning: {
		StatusCompleted:  {},
		StatusFailed:     {},
		StatusDeadLetter: {},
	},
	StatusFailed: {
		StatusPending:    {},
		StatusDeadLetter: {},
	},
	StatusCompleted:  {},
	StatusDeadLetter: {},
	StatusCancelled:  {},
}

// CanTransitionTo reports whether s may legally move to target.
//
// Unknown source or target statuses always return false.
func (s Status) CanTransitionTo(target Status) bool {
	targets, ok := validTransitions[s]
	if !ok {
		return false
	}
	_, ok = targets[target]
	return ok
}
