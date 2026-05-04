MIGRATIONS_PATH=db/migrations

ifneq (,$(wildcard .env))
  include .env
  export
endif


DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable

## ── Docker Compose ───────────────────────────────────────────────

COMPOSE_FILE := deploy/docker-compose.yml
COMPOSE := docker compose --env-file .env -f $(COMPOSE_FILE)

.PHONY: up down down-v restart ps logs logs-f db-shell redis-shell

up: ## Start all containers (detached)
	$(COMPOSE) up -d

down: ## Stop and remove all containers
	$(COMPOSE) down

down-v: ## Stop containers and remove volumes (fresh start)
	$(COMPOSE) down -v

restart: down up ## Restart all containers

ps: ## Show running containers
	$(COMPOSE) ps

logs: ## Show container logs (last 100 lines)
	$(COMPOSE) logs --tail=100

logs-f: ## Follow container logs live
	$(COMPOSE) logs -f

db-shell: ## Open psql shell in postgres container
	$(COMPOSE) exec postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

redis-shell: ## Open redis-cli in redis container
	$(COMPOSE) exec redis redis-cli -a $(REDIS_PASSWORD)


## ── Migrations ───────────────────────────────────────────────────

new_migration:
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(name)

migrate_up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose up $(steps)

migrate_down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose down $(steps)

# migrate_up_all:
# 	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose up

# migrate_down_all:
# 	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose down

migrate_goto:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose goto $(version)

migrate_force:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose force $(version)

sqlc:
	sqlc generate
generate:
	go generate

swagger:
	swag init --parseDependency -g cmd/main.go

GOLANGCI_LINT_VERSION := v2.11.1

## ── Install / Setup ──────────────────────────────────────────────

.PHONY: tools
tools: ## Install golangci-lint (same version as CI)
	@if ! command -v golangci-lint >/dev/null 2>&1 || \
		[ "$$(golangci-lint version --short 2>/dev/null)" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	else \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) already installed"; \
	fi

.PHONY: hooks
hooks: ## Install git pre-push hook
	@cp scripts/pre-push .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "✅ pre-push hook installed"

## ── Lint (mirrors CI exactly) ────────────────────────────────────

.PHONY: lint
lint: ## Run golangci-lint + go vet (same as CI)
	golangci-lint run --timeout 5m
	go vet ./...
	@echo ""
	@echo "✅ Lint passed — safe to push"

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix where possible
	golangci-lint run --timeout 5m --fix
	go vet ./...

.PHONY: lint-new
lint-new: ## Only lint uncommitted changes (fast iteration)
	golangci-lint run --timeout 5m --new-from-rev=HEAD~1

## ── Test ─────────────────────────────────────────────────────────

.PHONY: test
test: ## Run unit tests (short mode, no external deps)
	go test -v -race -short ./...

.PHONY: test-integration
test-integration: ## Run all tests including integration
	go test -v -race -coverprofile=coverage.out ./...

## ── Dev (live reload) ────────────────────────────────────────────

.PHONY: dev dev-server dev-agent
dev: migrate_up ## Run migrations, then start server + agent with live reload
	@trap 'kill 0' SIGINT SIGTERM; \
	air -c .air.toml & \
	air -c .air.agent.toml & \
	wait

dev-server: ## Run server only with live reload
	air -c .air.toml

dev-agent: ## Run agent only with live reload
	air -c .air.agent.toml

## ── Build ────────────────────────────────────────────────────────

.PHONY: build
build: ## Build server binary
	go build -o bin/cashpilotx ./cmd



## ── Format ───────────────────────────────────────────────────────

.PHONY: fmt
fmt: ## Format code
	gofmt -w .
	goimports -w .

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

## ── Pre-push check (everything CI checks) ───────────────────────

.PHONY: check
check: tidy fmt lint test build ## Full pre-push check (mirrors CI)
	@echo ""
	@echo "════════════════════════════════════════"
	@echo "  ✅ All checks passed — safe to push"
	@echo "════════════════════════════════════════"

## ── Help ─────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

.PHONY: sqlc new_migration migrate_up migrate_down migrate_goto migrate_force swagger generate lint-new lint-fix check hooks tools up down down-v restart ps logs logs-f db-shell redis-shell dev dev-server dev-agent

