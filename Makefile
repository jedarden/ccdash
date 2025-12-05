.PHONY: build install test clean deps run help

# Binary name
BINARY_NAME=ccdash
BINARY_PATH=./bin/$(BINARY_NAME)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean

# Version - set via environment variable or defaults to git tag/dev
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build flags - inject version at build time
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

# Default target
all: build

## build: Build the application binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_PATH) ./cmd/ccdash
	@echo "Build complete: $(BINARY_PATH)"

## install: Install the application to $GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOINSTALL) $(LDFLAGS) ./cmd/ccdash
	@echo "Installation complete. Run with: $(BINARY_NAME)"

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	@echo "Tests complete"

## test-coverage: Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## deps: Download and verify dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOGET) -v ./...
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "Dependencies updated"

## run: Build and run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	$(BINARY_PATH)

## clean: Remove build artifacts and cached files
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

## fmt: Format all Go code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
	@echo "Format complete"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...
	@echo "Vet complete"

## lint: Run all quality checks (fmt, vet)
lint: fmt vet

## release: Build release binaries for all platforms (requires VERSION env var)
release:
	@if [ "$(VERSION)" = "dev" ]; then echo "Error: VERSION must be set (e.g., make release VERSION=v0.6.0)"; exit 1; fi
	@echo "Building release $(VERSION) for all platforms..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/ccdash-linux-amd64 ./cmd/ccdash
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o bin/ccdash-linux-arm64 ./cmd/ccdash
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/ccdash-darwin-amd64 ./cmd/ccdash
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o bin/ccdash-darwin-arm64 ./cmd/ccdash
	@cd bin && for f in ccdash-*; do sha256sum "$$f" > "$$f.sha256" 2>/dev/null || shasum -a 256 "$$f" > "$$f.sha256"; done
	@echo "Release binaries built in bin/"
	@ls -la bin/

## help: Display this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
