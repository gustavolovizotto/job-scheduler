# ADR-0002: Job lifecycle and state machine

- **Status:** Accepted
- **Date:** 2026-05-14
- **Deciders:** Gustavo Tesin
- **Related:** [ADR-0001](0001-folder-structure.md)

## Context

A job moves through several states between submission and final disposition.
We need a single, explicit lifecycle that:

1. Captures every meaningful state operators and users care about.
2. Rejects illegal moves at the type level so bugs surface in tests, not in
   prod data.
3. Is small enough that the whole graph fits on screen.

## Decision

Seven statuses, listed below with their meaning:

| Status        | Description                                           | Terminal? |
|---------------|-------------------------------------------------------|-----------|
| `scheduled`   | RunAt is in the future; not yet eligible to run.      | no        |
| `pending`     | Ready to run; waiting for a worker to pick it up.     | no        |
| `running`     | A worker holds the lease and is executing the handler.| no        |
| `completed`   | Handler returned nil.                                 | **yes**   |
| `failed`      | Handler returned a transient error; retry available.  | no        |
| `dead_letter` | Permanent error or out of retries.                    | **yes**   |
| `cancelled`   | User cancelled the job before it ran.                 | **yes**   |

### Transition graph

```
                      ┌──────────────────────┐
                      │                      ▼
   scheduled ──▶ pending ──▶ running ──▶ completed
       │           │           │  │
       │           │           │  └─▶ failed ──▶ pending  (retry)
       │           │           │            │
       │           │           │            └─▶ dead_letter
       │           │           └─▶ dead_letter
       │           ▼
       │       cancelled
       ▼
   cancelled
```

Allowed transitions (and only these):

- `scheduled → pending` &nbsp;&nbsp;— RunAt reached, dispatcher promotes.
- `scheduled → cancelled` — user cancels before run window.
- `pending → running` &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;— worker pickup.
- `pending → cancelled`   — user cancels while waiting.
- `running → completed` &nbsp;&nbsp;— handler success.
- `running → failed` &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;— transient handler error.
- `running → dead_letter`  — permanent error (e.g. `PermanentError`).
- `failed → pending` &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;— retry scheduled with backoff.
- `failed → dead_letter` &nbsp;— retries exhausted.

Terminal statuses (`completed`, `dead_letter`, `cancelled`) have no outgoing
transitions — they cannot be "un-finished" or replayed in place. A retry of a
dead_letter goes through the API and creates a *new* job.

### Why these particular states?

- **Separate `scheduled` and `pending`** so the dispatcher's polling query
  can target `pending` only and the index `(status, run_at, priority)`
  remains efficient.
- **`failed` is non-terminal.** Sidekiq calls this "retrying"; we keep
  `failed` because it reads more naturally in the dashboard and matches what
  the user sees in logs.
- **`dead_letter` is preserved**, not deleted, so operators can inspect
  and replay. Retention policy is a separate concern (out of scope here).
- **`cancelled` is explicit**, not "deleted". A deleted row loses audit
  history; users sometimes need to know *why* a job didn't run.

### Implementation notes

- The state machine lives in `internal/job/status.go` as a `map[Status]map[Status]struct{}`.
- `Status.CanTransitionTo(target)` is the canonical check; `Job.TransitionTo`
  uses it before mutating the entity and additionally maintains the
  `UpdatedAt`, `StartedAt`, and `CompletedAt` timestamps.
- The test `TestStatus_TransitionMatrix` re-encodes the matrix and walks
  every `(from, to)` pair. Any divergence between the documented matrix and
  the code fails CI loudly.
- Failed transitions return `ErrInvalidTransition` (wrapped with the
  offending source/target). Callers map this to HTTP 409 Conflict.

## Consequences

**Positive:**

- Anyone can read `status.go` in 60 seconds and know exactly what's legal.
- Illegal moves are impossible without the upper layer going around the
  state machine — and we have lint/test gates to discourage that.

**Negative:**

- Adding a new state means updating the matrix in three places: the const
  block, the `validTransitions` map, and `TestStatus_TransitionMatrix`. That
  friction is intentional — the state machine should not grow casually.

## Follow-up

- When persistence lands (M2), the SQL schema constrains `status` to this
  set via a `CHECK` or an enum. The application-level state machine remains
  the source of truth for transitions.
