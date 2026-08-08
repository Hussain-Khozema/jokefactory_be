# Makefile for JokeFactory API
# Run 'make help' to see available targets

.PHONY: help run build test lint fmt tidy clean docker-build docker-up docker-down migrate-up migrate-down migrate-status migrate-create deploy

# Default target
.DEFAULT_GOAL := help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOFMT=gofmt
GOMOD=$(GOCMD) mod

# All first-party Go files (excludes the vendored dependencies)
GOFILES=$(shell find . -type f -name '*.go' -not -path './vendor/*')

# Binary name
BINARY_NAME=jokefactory
BINARY_DIR=bin

# Docker
DOCKER_COMPOSE=docker compose

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run the application locally (loads .env if present)
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi; \
	$(GOCMD) run .

build: ## Build the application binary
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 $(GOBUILD) -o $(BINARY_DIR)/$(BINARY_NAME) .

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-short: ## Run tests (short mode, skip integration tests)
	$(GOTEST) -v -short ./...

coverage: test ## Run tests and show coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Base git ref used to grandfather pre-existing lint issues: only issues
# introduced relative to this ref's merge-base fail the build. Existing debt is
# cleaned up as each aggregate is rewritten in later refactor phases.
# CI overrides this to origin/main.
LINT_BASE_REF ?= main
GOBIN_DIR=$(shell go env GOPATH)/bin

lint: ## Run golangci-lint (only new issues vs LINT_BASE_REF)
	@command -v golangci-lint > /dev/null 2>&1 || test -x $(GOBIN_DIR)/golangci-lint || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)
	@PATH="$(GOBIN_DIR):$$PATH" golangci-lint run --new-from-merge-base=$(LINT_BASE_REF) ./...

lint-all: ## Run golangci-lint across the whole codebase (includes pre-existing debt)
	@PATH="$(GOBIN_DIR):$$PATH" golangci-lint run ./...

lint-fix: ## Run golangci-lint with auto-fix (only new issues vs LINT_BASE_REF)
	@PATH="$(GOBIN_DIR):$$PATH" golangci-lint run --fix --new-from-merge-base=$(LINT_BASE_REF) ./...

fmt: ## Format code
	$(GOFMT) -s -w $(GOFILES)

fmt-check: ## Check code formatting
	@test -z "$$($(GOFMT) -l $(GOFILES))" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)

tidy: ## Tidy go.mod and go.sum
	$(GOMOD) tidy

verify: fmt-check lint test ## Run all verification steps (format check, lint, test)

# Azure deploy (requires: az login with Contributor on the resource group)
AZURE_RESOURCE_GROUP ?= ai-joke-factory
ACR_NAME ?= aijokefactoryacr
CONTAINER_APP_NAME ?= jokefactory-api
IMAGE_NAME ?= jokefactory
IMAGE_TAG ?= $(shell git rev-parse HEAD)

deploy: ## Build in ACR and update Container App (SKIP_VERIFY=1 to skip make verify)
	@if [ "$(SKIP_VERIFY)" != "1" ]; then $(MAKE) verify; fi
	@echo "Building $(IMAGE_NAME):$(IMAGE_TAG) in ACR $(ACR_NAME)..."
	az acr build \
		-r "$(ACR_NAME)" \
		-g "$(AZURE_RESOURCE_GROUP)" \
		-t "$(IMAGE_NAME):$(IMAGE_TAG)" \
		-t "$(IMAGE_NAME):latest" \
		.
	@echo "Updating Container App $(CONTAINER_APP_NAME)..."
	az containerapp update \
		-g "$(AZURE_RESOURCE_GROUP)" \
		-n "$(CONTAINER_APP_NAME)" \
		--image "$(ACR_NAME).azurecr.io/$(IMAGE_NAME):$(IMAGE_TAG)"
	@echo "Deployed $(ACR_NAME).azurecr.io/$(IMAGE_NAME):$(IMAGE_TAG)"

clean: ## Remove build artifacts
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html

# Docker targets
docker-build: ## Build Docker image
	docker build -t $(BINARY_NAME):latest .

docker-up: ## Start all services with Docker Compose
	$(DOCKER_COMPOSE) up -d

docker-down: ## Stop all services
	$(DOCKER_COMPOSE) down

docker-logs: ## Show logs from all services
	$(DOCKER_COMPOSE) logs -f

docker-restart: docker-down docker-up ## Restart all services

psql: ## Open psql shell inside the postgres container
	$(DOCKER_COMPOSE) exec postgres psql -U postgres -d jokefactory

# Database migrations (using goose)
# Requires: go install github.com/pressly/goose/v3/cmd/goose@latest
MIGRATIONS_DIR=src/infra/db/migrations
DB_DSN?=postgres://postgres:postgres@localhost:5432/jokefactory?sslmode=disable

migrate-up: ## Run all pending migrations
	@which goose > /dev/null || (echo "Installing goose..." && go install github.com/pressly/goose/v3/cmd/goose@latest)
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" up

migrate-down: ## Rollback the last migration
	@which goose > /dev/null || (echo "Installing goose..." && go install github.com/pressly/goose/v3/cmd/goose@latest)
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" down

migrate-status: ## Show migration status
	@which goose > /dev/null || (echo "Installing goose..." && go install github.com/pressly/goose/v3/cmd/goose@latest)
	goose -dir $(MIGRATIONS_DIR) postgres "$(DB_DSN)" status

migrate-create: ## Create a new migration (usage: make migrate-create name=add_users_table)
	@which goose > /dev/null || (echo "Installing goose..." && go install github.com/pressly/goose/v3/cmd/goose@latest)
	goose -dir $(MIGRATIONS_DIR) create $(name) sql

# Development helpers
dev-deps: ## Install development dependencies
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	@echo "Development dependencies installed"

.PHONY: generate
generate: ## Run go generate
	$(GOCMD) generate ./...

restart: 
	$(DOCKER_COMPOSE) build api
	$(DOCKER_COMPOSE) up -d --force-recreate