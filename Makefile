.PHONY: help all run test coverage cover lint lint-ci fmt vet build build-nocheck build-all release-all install clean

help: ## This help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

APP_NAME       = modbusctl
APP_SRC	= main.go
BIN_DIR        = bin
BINARY         = $(BIN_DIR)/$(APP_NAME)
ARCHS	  = linux/amd64 linux/arm64 linux/arm/v7 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# Override on the command line or in CI (matches reusable release workflow ldflags).
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
TAG        ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.tag=$(TAG)' -X 'main.commit=$(COMMIT)' -X 'main.buildDate=$(BUILD_DATE)'"
RELEASE_DIR   := release

all: test build build-all ## Test and build the application

run: ## Run the application
	@echo "Running $(APP_NAME)"
	@go run $(APP_SRC)

check: fmt lint lint-ci vet test ## Run all checks (format, lint, vet, test)

test: ## Run unit tests with race detector
	@echo "Running unit tests (race detector)"
	@go test -count=1 -race ./...

coverage: ## Run tests with coverage (writes coverage.out)
	@echo "Running tests with coverage"
	@go test -count=1 -race -coverprofile=coverage.out -covermode=atomic ./...

cover: coverage ## Open coverage report in browser (run coverage first)
	@echo "Opening coverage report in browser"
	@go tool cover -html=coverage.out

lint: ## Run staticcheck
	@echo "Running staticcheck"
	@staticcheck ./...

lint-ci: ## Run golangci-lint; uses .golangci.yml (enables unparam for unused-param checks)
	@echo "Running golangci-lint"
	@golangci-lint run ./...

fmt: ## Format Go code with gofmt
	@echo "Running gofmt"
	@gofmt -w .

vet: ## Run go vet on project packages
	@echo "Running go vet"
	@go vet ./...

build: check ## Build the application
	@echo "Building $(BINARY)"
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BINARY) $(APP_SRC)

build-nocheck: fmt lint lint-ci vet ## Build without running tests (e.g. Docker where -race needs CGO)
	@echo "Building $(BINARY)"
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BINARY) $(APP_SRC)

build-all: ## Build the application for all architectures
	@mkdir -p $(RELEASE_DIR)
	@for arch in $(ARCHS); do \
		os=$${arch%%/*}; \
		rest=$${arch#*/}; \
		cpu=$${rest%%/*}; \
		variant=$${rest#*/}; \
		if [ "$$os" = "windows" ]; then \
			echo "Building $(APP_NAME)-$(VERSION)-$$os-$$cpu.exe..."; \
			GOOS=$$os GOARCH=$$cpu go build $(LDFLAGS) -o $(RELEASE_DIR)/$(APP_NAME)-$(VERSION)-$$os-$$cpu.exe $(APP_SRC); \
		elif [ "$$cpu" = "arm" ] && [ "$$variant" = "v7" ]; then \
			echo "Building $(APP_NAME)-$(VERSION)-$$os-armv7..."; \
			GOOS=$$os GOARCH=$$cpu GOARM=7 go build $(LDFLAGS) -o $(RELEASE_DIR)/$(APP_NAME)-$(VERSION)-$$os-armv7 $(APP_SRC); \
		else \
			echo "Building $(APP_NAME)-$(VERSION)-$$os-$$cpu..."; \
			GOOS=$$os GOARCH=$$cpu go build $(LDFLAGS) -o $(RELEASE_DIR)/$(APP_NAME)-$(VERSION)-$$os-$$cpu $(APP_SRC); \
		fi \
	done

release-all: build-all ## Package the build binaries into tar.gz (Unix) or zip (Windows) for all architectures
	@mkdir -p $(RELEASE_DIR)
	@for arch in $(ARCHS); do \
		os=$${arch%%/*}; \
		rest=$${arch#*/}; \
		cpu=$${rest%%/*}; \
		variant=$${rest#*/}; \
		if [ "$$os" = "windows" ]; then \
			bin=$(APP_NAME)-$(VERSION)-$$os-$$cpu.exe; \
			echo "Packaging $$bin into $(APP_NAME)-$(VERSION)-$$os-$$cpu.zip..."; \
			cd $(RELEASE_DIR) && zip -q $(APP_NAME)-$(VERSION)-$$os-$$cpu.zip $$bin && cd - > /dev/null; \
		elif [ "$$cpu" = "arm" ] && [ "$$variant" = "v7" ]; then \
			bin=$(APP_NAME)-$(VERSION)-$$os-armv7; \
			echo "Packaging $$bin.tar.gz..."; \
			tar czf $(RELEASE_DIR)/$$bin.tar.gz -C $(RELEASE_DIR) $$bin; \
		else \
			bin=$(APP_NAME)-$(VERSION)-$$os-$$cpu; \
			echo "Packaging $$bin.tar.gz..."; \
			tar czf $(RELEASE_DIR)/$$bin.tar.gz -C $(RELEASE_DIR) $$bin; \
		fi; \
	done

install: build ## Install the built binary to /usr/local/bin
	@echo "Installing $(APP_NAME) to /usr/local/bin"
	@sudo install -m 0755 $(BINARY) /usr/local/bin/$(APP_NAME)

clean: ## Clean the build artifacts
	@echo "Cleaning build artifacts"
	@rm -rf $(BIN_DIR)
	@rm -f $(BOOTSTRAP_NAME)
	@rm -f $(RELEASE_DIR)/*
	@rm -f *.{json,csv,log,mcap} results/*.{json,csv,log,mcap}