# Linko Build Configuration

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=linko
BINARY_UNIX=$(BINARY_NAME)_unix

# UI parameters
UI_DIR=pkg/ui
BUN ?= bun

# Build flags
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X github.com/monsterxx03/linko/pkg/version.Version=$(VERSION) -X github.com/monsterxx03/linko/pkg/version.Commit=$$(git rev-parse --short HEAD) -X github.com/monsterxx03/linko/pkg/version.Date=$$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Default target
all: ui-build build

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

# Build the binary
build:
	@echo "Building binary..."
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/linko

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_UNIX) ./cmd/linko

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf bin/

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Dev tools
dev-deps:
	@echo "Installing development tools..."
	$(GOCMD) install golang.org/x/tools/cmd/goimports
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint

# Format code
fmt:
	@echo "Formatting code..."
	goimports -w .

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# UI targets
ui-deps:
	@echo "Installing UI dependencies..."
	cd $(UI_DIR) && $(BUN) install

ui-dev:
	@echo "Starting UI dev server..."
	cd $(UI_DIR) && $(BUN) run dev

ui-build:
	@echo "Building UI..."
	cd $(UI_DIR) && $(BUN) run build

ui-preview:
	@echo "Previewing UI build..."
	cd $(UI_DIR) && $(BUN) run preview

# Help
help:
	@echo "Available targets:"
	@echo "  all          - Clean, deps, test, build"
	@echo "  deps         - Install Go dependencies"
	@echo "  build        - Build the binary"
	@echo "  build-linux  - Build for Linux"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  dev-deps     - Install development tools"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  ui-deps      - Install UI dependencies"
	@echo "  ui-dev       - Start UI dev server (http://localhost:5173)"
	@echo "  ui-build     - Build UI to dist/admin"
	@echo "  ui-preview   - Preview UI build"
	@echo "  ui           - Build UI and Go binary with embedded UI"
	@echo "  help         - Show this help"
