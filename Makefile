# Vergex Makefile for testing and development

.PHONY: help test test-backend test-frontend test-coverage build build-backend build-frontend \
	dev run-backend run-frontend run-postgresql fmt fmt-backend fmt-frontend lint lint-backend lint-frontend \
	init up down restart logs status regen-keys deps deps-backend deps-frontend deps-update clean

# Default target
help:
	@echo "Vergex Testing & Development Commands"
	@echo ""
	@echo "Testing:"
	@echo "  make test                 - Run all tests (backend + frontend)"
	@echo "  make test-backend         - Run backend tests only"
	@echo "  make test-frontend        - Run frontend tests only"
	@echo "  make test-coverage        - Generate backend coverage report"
	@echo ""
	@echo "Build:"
	@echo "  make build                - Build backend binary and frontend"
	@echo "  make build-backend        - Build backend binary"
	@echo "  make build-frontend       - Build frontend"
	@echo ""
	@echo "Development:"
	@echo "  make dev                  - Run backend and frontend dev server together"
	@echo "  make run-backend          - Run backend (go run)"
	@echo "  make run-frontend         - Run frontend dev server"
	@echo "  make run-postgresql       - Start PostgreSQL with the settings in .env"
	@echo "  make fmt                  - Format backend and frontend code"
	@echo "  make lint                 - Lint backend and frontend code"
	@echo ""
	@echo "Deployment (docker compose):"
	@echo "  make init                 - Create .env and fill missing keys"
	@echo "  make up                   - init, then build and start containers"
	@echo "  make down                 - Stop and remove containers"
	@echo "  make restart              - Restart containers"
	@echo "  make logs [SERVICE=...]   - Follow logs (vergex-backend or vergex-frontend)"
	@echo "  make status               - Container status and health check"
	@echo "  make regen-keys           - Regenerate encryption keys (destructive)"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps                 - Install backend and frontend dependencies"
	@echo "  make deps-update          - Update Go dependencies"
	@echo ""
	@echo "Clean:"
	@echo "  make clean                - Clean build artifacts and test cache"

# =============================================================================
# Testing
# =============================================================================

# Run all tests
test: test-backend test-frontend
	@echo "✅ All tests completed"

# Backend tests only
test-backend: run-postgresql
	@echo "🧪 Running backend tests..."
	go test -v ./...

# Frontend tests only
test-frontend:
	@echo "🧪 Running frontend tests..."
	cd web && pnpm test

# Coverage report
test-coverage: run-postgresql
	@echo "📊 Generating coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Backend coverage: coverage.html"

# =============================================================================
# Build
# =============================================================================

# Build backend binary and frontend
build: build-backend build-frontend

# Build backend binary
build-backend:
	@echo "🔨 Building backend..."
	go build -o vergex ./cmd/vergex
	@echo "✅ Backend built: ./vergex"

# Build frontend
build-frontend:
	@echo "🔨 Building frontend..."
	cd web && pnpm build
	@echo "✅ Frontend built: ./web/dist"

# =============================================================================
# Development
# =============================================================================

# Run backend and frontend dev server in parallel (Ctrl-C stops both)
dev:
	@$(MAKE) -j2 run-backend run-frontend

# Run backend in development mode, with the settings in .env
run-backend: run-postgresql
	@echo "🚀 Starting backend..."
	go run ./cmd/vergex

# Run frontend in development mode
run-frontend:
	@echo "🚀 Starting frontend dev server..."
	cd web && pnpm dev

# Start PostgreSQL with the settings in .env, on 127.0.0.1 at DB_PORT
run-postgresql: init
	@scripts/run-postgresql.sh

# Format backend and frontend code
fmt: fmt-backend fmt-frontend

fmt-backend:
	@echo "🎨 Formatting Go code..."
	go fmt ./...

fmt-frontend:
	@echo "🎨 Formatting frontend code..."
	cd web && pnpm format

# Lint backend and frontend code
lint: lint-backend lint-frontend

# Lint Go code (requires golangci-lint)
lint-backend:
	@echo "🔍 Linting Go code..."
	golangci-lint run

lint-frontend:
	@echo "🔍 Linting frontend code..."
	cd web && pnpm lint

# =============================================================================
# Deployment (docker compose, local build)
# =============================================================================

# Create .env from .env.example and fill missing keys (existing values are kept)
init:
	@scripts/env-keys.sh init

# Build images and start the stack. Inside it the backend reaches the database over
# the compose network, whatever DB_HOST and DB_PORT in .env say for running from source.
up: init
	DB_HOST=vergex-postgres DB_PORT=5432 docker compose up -d --build

# Stop and remove the containers (data/ is kept)
down:
	docker compose down

# Restart the services
restart:
	docker compose restart

# Follow logs; pick one service with SERVICE=vergex-backend or SERVICE=vergex-frontend
logs:
	docker compose logs -f $(SERVICE)

# Container status and health through the frontend proxy
status:
	@docker compose ps
	@port=$$(grep -E '^VERGEX_FRONTEND_PORT=' .env 2>/dev/null | cut -d= -f2 | tr -d "\"' "); \
	addr=$$(grep -E '^VERGEX_BIND_ADDRESS=' .env 2>/dev/null | cut -d= -f2 | tr -d "\"' "); \
	url="http://$${addr:-127.0.0.1}:$${port:-8000}/api/health"; \
	echo "Health: $$url"; curl -fsS "$$url" && echo || echo "Backend not responding"

# Regenerate all encryption keys (asks for confirmation; old encrypted data becomes unreadable)
regen-keys:
	@scripts/env-keys.sh regen

# =============================================================================
# Dependencies
# =============================================================================

# Install backend and frontend dependencies
deps: deps-backend deps-frontend

# Download Go dependencies
deps-backend:
	@echo "📦 Downloading Go dependencies..."
	go mod download
	@echo "✅ Dependencies downloaded"

# Install frontend dependencies
deps-frontend:
	@echo "📦 Installing frontend dependencies..."
	cd web && pnpm install
	@echo "✅ Frontend dependencies installed"

# Update Go dependencies
deps-update:
	@echo "📦 Updating Go dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✅ Dependencies updated"

# =============================================================================
# Clean
# =============================================================================

clean:
	@echo "🧹 Cleaning..."
	rm -f vergex
	rm -f coverage.out coverage.html
	rm -rf web/dist
	go clean -testcache
	@echo "✅ Cleaned"
