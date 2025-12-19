# Makefile for otlpxy
# Supports: Development, Testing, Docker (multi-arch)

# ============================================================================
# Variables
# ============================================================================

# Application
APP_NAME := otlpxy
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Paths
CMD_PATH := ./cmd/server
BIN_DIR := ./bin

# Go
GO := go
GOFLAGS := -v
LDFLAGS := -ldflags="-w -s -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.Commit=$(COMMIT)"

# Docker
DOCKER := docker
DOCKER_BUILDX := $(DOCKER) buildx
IMAGE_NAME := otlpxy
IMAGE_TAG := $(VERSION)
IMAGE_LATEST := latest

# Local platform detection for docker --load (single-arch only)
UNAME_M := $(shell uname -m)
LOCAL_ARCH := $(if $(filter $(UNAME_M),x86_64 amd64),amd64,$(if $(filter $(UNAME_M),arm64 aarch64),arm64,amd64))
LOCAL_PLATFORM := linux/$(LOCAL_ARCH)

# Colors for output
CYAN := \033[0;36m
GREEN := \033[0;32m
RED := \033[0;31m
YELLOW := \033[0;33m
NC := \033[0m # No Color

# ============================================================================
# Phony Targets
# ============================================================================

.PHONY: all build test deps vendor tidy update docker-build docker-run clean help run

# ============================================================================
# Default Target
# ============================================================================

all: clean test build

# ============================================================================
# Development Commands
# ============================================================================

## build: Build binary for current platform
build:
	@echo "$(CYAN)Building $(APP_NAME)...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME) $(CMD_PATH)
	@echo "$(GREEN)✓ Binary built: $(BIN_DIR)/$(APP_NAME)$(NC)"

## build-loadtest: Build load test binary for current platform
build-loadtest:
	@echo "$(CYAN)Building loadtest binary...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/loadtest ./loadtest.go
	@echo "$(GREEN)✓ Binary built: $(BIN_DIR)/loadtest$(NC)"

## run: Build and run the application
run: build
	@echo "$(CYAN)Running $(APP_NAME)...$(NC)"
	$(BIN_DIR)/$(APP_NAME)

# ============================================================================
# Testing Commands
# ============================================================================

## test: Run all tests
test:
	@echo "$(CYAN)Running tests...$(NC)"
	$(GO) test -race -short ./...
	@echo "$(GREEN)✓ Tests passed$(NC)"


# ============================================================================
# Dependency Management
# ============================================================================

## deps: Download dependencies
deps:
	@echo "$(CYAN)Downloading dependencies...$(NC)"
	$(GO) mod download
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

## vendor: Create vendor directory
vendor:
	@echo "$(CYAN)Creating vendor directory...$(NC)"
	$(GO) mod vendor
	@echo "$(GREEN)✓ Vendor directory created$(NC)"

## tidy: Clean up go.mod and go.sum
tidy:
	@echo "$(CYAN)Tidying dependencies...$(NC)"
	$(GO) mod tidy
	@echo "$(GREEN)✓ Dependencies tidied$(NC)"

## update: Update dependencies
update:
	@echo "$(CYAN)Updating dependencies...$(NC)"
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "$(GREEN)✓ Dependencies updated$(NC)"

# ============================================================================
# Docker Commands
# ============================================================================

## docker-build: Build Docker image for the current host platform (loads locally)
docker-build:
	@echo "$(CYAN)Building Docker image for local platform...$(NC)"
	@echo "$(YELLOW)Platform: $(LOCAL_PLATFORM)$(NC)"
	$(DOCKER_BUILDX) build \
		--platform $(LOCAL_PLATFORM) \
		--tag $(IMAGE_NAME):$(IMAGE_TAG) \
		--tag $(IMAGE_NAME):$(IMAGE_LATEST) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg COMMIT=$(COMMIT) \
		--load \
		.
	@echo "$(GREEN)✓ Docker image built and loaded locally$(NC)"

## docker-run: Run Docker container locally
docker-run:
	@echo "$(CYAN)Running Docker container...$(NC)"
	$(DOCKER) run -d \
		--name $(APP_NAME) \
		-p 8080:8080 \
		-e OTEL_COLLECTOR_TARGET_URL=https://otel.zep.us \
		-e OTEL_COLLECTOR_API_KEY=$${OTEL_COLLECTOR_API_KEY:-} \
		-e ALLOWED_ORIGINS=https://quiz.zep.us,https://school.zep.us \
		$(IMAGE_NAME):$(IMAGE_LATEST)
	@echo "$(GREEN)✓ Container running: $(APP_NAME)$(NC)"
	@echo "$(YELLOW)Health check: curl http://localhost:8080/healthz$(NC)"

docker-down:
	@echo "$(CYAN)Stopping and removing Docker container...$(NC)"
	$(DOCKER) stop $(APP_NAME)
	$(DOCKER) rm $(APP_NAME)
	@echo "$(GREEN)✓ Container stopped and removed$(NC)"


# ============================================================================
# Utility Commands
# ============================================================================

## clean: Remove build artifacts and cache
clean:
	@echo "$(CYAN)Cleaning build artifacts...$(NC)"
	-rm -rf $(BIN_DIR)
	-rm -rf coverage/
	-rm -rf vendor/
	-$(GO) clean -cache -testcache
	@echo "$(GREEN)✓ Cleaned$(NC)"

## help: Show this help message
help:
	@echo "$(CYAN)Zep Logger - Makefile Commands$(NC)"
	@echo ""
	@echo "$(YELLOW)Development:$(NC)"
	@echo "  make build           - Build binary for current platform"
	@echo "  make build-loadtest  - Build load test binary"
	@echo "  make run             - Build and run the application"
	@echo ""
	@echo "$(YELLOW)Testing:$(NC)"
	@echo "  make test            - Run all tests"

	@echo "$(YELLOW)Dependencies:$(NC)"
	@echo "  make deps            - Download dependencies"
	@echo "  make vendor          - Create vendor directory"
	@echo "  make tidy            - Clean up go.mod and go.sum"
	@echo "  make update          - Update dependencies"
	@echo ""
	@echo "$(YELLOW)Docker:$(NC)"
	@echo "  make docker-build    - Build local Docker image (current platform)"
	@echo "  make docker-run      - Run Docker container locally"
	@echo ""
	@echo "$(YELLOW)Utility:$(NC)"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make help            - Show this help"
	@echo ""
	@echo "$(YELLOW)Info:$(NC)"
	@echo "  Version: $(VERSION)"
	@echo "  Commit:  $(COMMIT)"
