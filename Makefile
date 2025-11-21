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

# Build flags
LDFLAGS=-ldflags "-s -w"

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

## help: Display this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
