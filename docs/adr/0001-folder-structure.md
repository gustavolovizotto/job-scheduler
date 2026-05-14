# ADR-0001: Folder structure and import conventions

- **Status:** Accepted
- **Date:** 2026-05-14
- **Deciders:** Gustavo Tesin

## Context

We need a folder layout that:

1. Is idiomatic for the Go community (so a recruiter/contributor opens the repo and immediately understands it).
2. Enforces dependency rules — domain logic must not depend on infrastructure.
3. Allows the project to grow into multiple binaries (scheduler API + workers, separate migrate CLI, possibly future tools) without restructuring.
4. Makes integration tests easy to colocate with the code they test.

There are three common conventions in the Go ecosystem:

- **Flat package layout** — small projects, everything in the root. Fine for libraries but breaks down quickly when you have an API + workers + storage.
- **[Standard Go Project Layout](https://github.com/golang-standards/project-layout)** — `cmd/`, `internal/`, `pkg/`. Widely understood; mildly controversial but the closest thing to a community default.
- **Clean / hexagonal architecture** — split by layer (domain, application, infrastructure, ports).

## Decision

We adopt a **hybrid**: the *outer shape* follows the Standard Go Project Layout (`cmd/`, `internal/`, `pkg/`, `migrations/`, `docs/`, `deployments/`, `scripts/`), and **inside `internal/`** we organize packages by **bounded context / responsibility** rather than by technical layer.

```
.
├── cmd/                          # Binary entry points (one subdir per binary)
│   ├── scheduler/                #   Main binary — HTTP API + worker pool
│   └── migrate/                  #   CLI for applying SQL migrations
├── internal/                     # Application code — NOT importable from outside
│   ├── api/                      #   HTTP handlers, middleware, router, DTOs
│   ├── job/                      #   Domain: Job entity, Status, JobType, state machine
│   ├── scheduler/                #   Dispatcher, recurring jobs, leader election
│   ├── worker/                   #   Worker pool, executor, handler registry
│   ├── storage/                  #   Repository interfaces + Postgres implementations
│   ├── observability/            #   slog setup, Prometheus metrics, OTel tracing
│   └── config/                   #   Typed config struct + env loading
├── pkg/                          # Reusable, exported libraries
│   └── (e.g. client/)            #   Go client SDK — safe for third-party use
├── migrations/                   # SQL migrations (golang-migrate format)
├── docs/
│   └── adr/                      # Architecture Decision Records (this folder)
├── deployments/                  # Dockerfile, docker-compose.yml, K8s manifests
└── scripts/                      # Helper scripts (bench, seed, dev tooling)
```

### Why this hybrid?

- **`cmd/` keeps `main.go` thin.** Each binary's main is just wiring — it imports from `internal/` and starts things up. This makes it trivial to add a second binary later.
- **`internal/` is enforced by the Go compiler.** Nothing outside this module can import these packages, so the public API surface stays small (only `pkg/`).
- **Package-by-feature inside `internal/`.** A "clean architecture" with `domain/`, `application/`, `infrastructure/` folders would over-engineer this codebase. Splitting by responsibility (`job/`, `worker/`, `storage/`) reads more naturally and matches how Go developers actually think.
- **`pkg/` is reserved for the client SDK.** We only put code there if a third party would legitimately import it.

## Dependency rules

To keep the design honest, packages should only depend "inward":

```
api  ──▶ scheduler ──▶ worker ──▶ storage ──▶ job
                          │
                          ▼
                  observability, config (used by all)
```

- `job/` is the innermost package. It has **no dependencies on other internal packages** — only stdlib + `uuid`. It owns the `Job` entity, statuses, the state machine, and domain errors.
- `storage/` defines the `JobRepository` interface and provides the Postgres implementation. It depends on `job/` (the entity it persists).
- `worker/` depends on `job/` and `storage/` (it reads/writes jobs through the repo).
- `scheduler/` orchestrates the worker pool; depends on `worker/` and `storage/`.
- `api/` is the outermost; it depends on `scheduler/`, `storage/`, `job/`.
- `observability/` and `config/` are leaf utilities that other packages may import freely.

**The forbidden direction:** `job/` must never import from `storage/`, `api/`, or anywhere else in `internal/`. This is what makes domain tests fast and decoupled.

## Import conventions

- **Local prefix.** `goimports` is configured (in `.golangci.yml`) with `local-prefixes: github.com/gustavotesin/go-job-scheduler` so internal imports are grouped separately from stdlib and third-party imports.
- **Three groups, blank-line separated:**

  ```go
  import (
      "context"
      "fmt"

      "github.com/google/uuid"
      "github.com/jackc/pgx/v5"

      "github.com/gustavotesin/go-job-scheduler/internal/job"
      "github.com/gustavotesin/go-job-scheduler/internal/storage"
  )
  ```

- **Package naming.** Lowercase, single word, no underscores or camelCase. `package job`, not `package jobs` or `package jobDomain`.
- **No relative imports.** Always use the full module path.
- **Test files** live next to the code they test (`job/job_test.go`) and use the `_test` suffix only when testing the public API from the outside (`job/job_external_test.go` with `package job_test`).

## Consequences

**Positive:**

- A newcomer can navigate the repo in under 60 seconds.
- The Go compiler enforces the most important boundary (`internal/`).
- Adding a new binary or a new bounded context is a folder-level operation, not a rewrite.
- Dependency direction is documented and enforceable via tools (e.g. `go-cleanarch`, `depguard` linter).

**Negative:**

- The `pkg/` vs `internal/` distinction is a frequent source of debate in the Go community; we settle it by being strict: only the client SDK ever moves to `pkg/`.
- Package-by-feature can lead to large packages over time; we'll watch for this and split if a package crosses ~1500 LOC.

## Follow-up

- Once `internal/job/` exists, add a `depguard` rule to forbid imports from `internal/storage/` or `internal/api/` into `internal/job/`.
- Add a CI step that runs `go-cleanarch` or equivalent.
