# Go Job Scheduler — Makefile

BINARY_NAME      := scheduler
BUILD_DIR        := bin
DOCKER_IMAGE     := go-job-scheduler
DOCKER_TAG       := latest
MIGRATIONS_DIR   := migrations
DB_URL           ?= postgres://postgres:postgres@localhost:5432/scheduler?sslmode=disable

GO               ?= go
GOFLAGS          ?=
LDFLAGS          := -s -w

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## ────── Development ──────

.PHONY: run
run: ## Run the scheduler locally
	$(GO) run ./cmd/scheduler

.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

## ────── Tests ──────

.PHONY: test
test: ## Run unit tests with race detector
	$(GO) test -race -cover ./...

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker)
	$(GO) test -race -tags=integration -timeout=5m ./...

.PHONY: coverage
coverage: ## Generate HTML coverage report
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## ────── Quality ──────

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format code with gofmt
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

## ────── Build ──────

.PHONY: build
build: ## Build the binary into ./bin
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scheduler

.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f deployments/Dockerfile .

## ────── Database ──────

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

.PHONY: migrate-down
migrate-down: ## Rollback the last migration
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create a new migration (use NAME=foo)
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=migration_name" && exit 1)
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)

## ────── Misc ──────

.PHONY: install-hooks
install-hooks: ## Install git pre-commit hook
	git config core.hooksPath .githooks
	@echo "✓ Pre-commit hook installed"

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR) coverage.out coverage.html

.DEFAULT_GOAL := help
