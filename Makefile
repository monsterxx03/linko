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

# Build flags
LDFLAGS=-ldflags "-X github.com/monsterxx03/linko/pkg/dns.Version=0.1.0"

# Default target
all: clean deps test build

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

# Install to GOPATH
install:
	@echo "Installing..."
	$(GOCMD) install $(LDFLAGS) ./cmd/linko

# Run the application
run:
	@echo "Running..."
	$(GOCMD) run ./cmd/linko/main.go

# Download GeoIP database
download-geoip:
	@echo "Downloading GeoIP database..."
	@mkdir -p data
	@if [ ! -f .env ]; then echo "Error: .env file not found. Copy .env.example to .env and add your credentials."; exit 1; fi
	@. .env && if [ -z "$$MAXMIND_ACCOUNT_ID" ] || [ -z "$$MAXMIND_LICENSE_KEY" ]; then echo "Error: MAXMIND_ACCOUNT_ID or MAXMIND_LICENSE_KEY not set in .env"; exit 1; fi
	@. .env && curl -L -o /tmp/geoip.tar.gz "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=$$MAXMIND_LICENSE_KEY&suffix=tar.gz" --user "$$MAXMIND_ACCOUNT_ID:$$MAXMIND_LICENSE_KEY"
	@tar -xzf /tmp/geoip.tar.gz -C /tmp
	@mv /tmp/GeoLite2-Country_*/GeoLite2-Country.mmdb data/geoip.mmdb 2>/dev/null || mv /tmp/*/GeoLite2-Country.mmdb data/geoip.mmdb
	@rm -rf /tmp/geoip.tar.gz /tmp/GeoLite2-Country_* 2>/dev/null
	@echo "GeoIP database downloaded to data/geoip.mmdb"

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

# Check for security vulnerabilities
security:
	@echo "Checking for security vulnerabilities..."
	$(GOCMD) install github.com/securecodewarrior/gosec/v2/cmd/gosec
	gosec ./...

# Benchmark tests
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Profile CPU usage
profile:
	@echo "Profiling CPU usage..."
	$(GOTEST) -cpuprofile=cpu.prof -memprofile=mem.prof -bench=. ./...
	@echo "Profile files: cpu.prof, mem.prof"

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t linko:latest .

# Docker run
docker-run:
	@echo "Running Docker container..."
	docker run -p 7890:7890 -p 5353:5353 -v $(PWD)/config:/app/config linko:latest

# Help
help:
	@echo "Available targets:"
	@echo "  all          - Clean, deps, test, build"
	@echo "  deps         - Install dependencies"
	@echo "  build        - Build the binary"
	@echo "  build-linux  - Build for Linux"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  install      - Install to GOPATH"
	@echo "  run          - Run the application"
	@echo "  download-geoip - Download GeoIP database"
	@echo "  dev-deps     - Install development tools"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  security     - Check security vulnerabilities"
	@echo "  bench        - Run benchmarks"
	@echo "  profile      - Profile CPU usage"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  help         - Show this help"
