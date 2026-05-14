package job

import (
	"testing"
)

func TestStatus_IsValid(t *testing.T) {
	for _, s := range AllStatuses() {
		if !s.IsValid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []Status{"", "unknown", "PENDING", "Running"} {
		if s.IsValid() {
			t.Errorf("%q should NOT be valid", s)
		}
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	terminal := map[Status]bool{
		StatusCompleted:  true,
		StatusDeadLetter: true,
		StatusCancelled:  true,
	}
	for _, s := range AllStatuses() {
		got := s.IsTerminal()
		want := terminal[s]
		if got != want {
			t.Errorf("%q.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

// TestStatus_TransitionMatrix exhaustively asserts every (from, to) pair
// against the documented state machine. The expected matrix is duplicated
// here intentionally — if someone changes validTransitions in status.go,
// this test must also be updated, which keeps documentation and code aligned.
func TestStatus_TransitionMatrix(t *testing.T) {
	allowed := map[Status]map[Status]bool{
		StatusScheduled: {
			StatusPending:   true,
			StatusCancelled: true,
		},
		StatusPending: {
			StatusRunning:   true,
			StatusCancelled: true,
		},
		StatusRunning: {
			StatusCompleted:  true,
			StatusFailed:     true,
			StatusDeadLetter: true,
		},
		StatusFailed: {
			StatusPending:    true,
			StatusDeadLetter: true,
		},
		// terminals — no outgoing transitions
		StatusCompleted:  {},
		StatusDeadLetter: {},
		StatusCancelled:  {},
	}

	for _, from := range AllStatuses() {
		for _, to := range AllStatuses() {
			want := allowed[from][to]
			got := from.CanTransitionTo(to)
			if got != want {
				t.Errorf("CanTransitionTo(%q → %q) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestStatus_TerminalStatusesHaveNoOutgoing(t *testing.T) {
	for _, term := range []Status{StatusCompleted, StatusDeadLetter, StatusCancelled} {
		for _, to := range AllStatuses() {
			if term.CanTransitionTo(to) {
				t.Errorf("terminal %q should not transition to %q", term, to)
			}
		}
	}
}

func TestStatus_UnknownSourceCannotTransition(t *testing.T) {
	unknown := Status("gibberish")
	for _, to := range AllStatuses() {
		if unknown.CanTransitionTo(to) {
			t.Errorf("unknown status should not transition to %q", to)
		}
	}
}

func TestStatus_CannotTransitionToUnknownTarget(t *testing.T) {
	for _, from := range AllStatuses() {
		if from.CanTransitionTo("gibberish") {
			t.Errorf("%q should not transition to unknown target", from)
		}
	}
}
