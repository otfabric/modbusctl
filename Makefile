.PHONY: help all run test coverage cover compile lint lint-ci fmt fmt-check vet build build-nocheck build-all release-all install clean check-legacy

help: ## This help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

# Parent otfabric/go.work does not list this module; isolate toolchain commands.
export GOWORK := off

# CI formats and lints with the Go version from go.mod, not with the toolchain installed here.
# gofmt's output changes between Go releases (comment alignment, for one), so formatting is
# done with that version, and `fmt-check` also requires the local gofmt to agree: a file that
# two versions format differently fails CI on one side or the other.
GO_MOD_VERSION := $(shell sed -nE 's/^go ([0-9]+\.[0-9]+).*/\1/p' go.mod | head -n1)
GOFMT_CI = $(shell GOTOOLCHAIN=go$(GO_MOD_VERSION).0 go env GOROOT)/bin/gofmt

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

# Minimum total statement coverage (percent) enforced by `make coverage`.
COVERAGE_MIN ?= 85

all: test build build-all ## Test and build the application

run: ## Run the application
	@echo "Running $(APP_NAME)"
	@go run $(APP_SRC)

check: fmt fmt-check lint lint-ci vet test coverage ## Run all checks (format, lint, vet, test, coverage)

test: ## Run unit tests with race detector
	@echo "Running unit tests (race detector)"
	@go test -count=1 -race ./...

coverage: ## Run tests with whole-module coverage (writes coverage.out); fails below COVERAGE_MIN percent
	@echo "Running tests with coverage"
	@go test -count=1 -race -coverprofile=coverage.out -covermode=atomic -coverpkg=./... ./...
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $$3); print $$3 }'); \
	echo "Total coverage: $$total% (minimum $(COVERAGE_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit (t + 0 < m + 0) ? 1 : 0 }' || \
		{ echo "Coverage $$total% is below the required $(COVERAGE_MIN)%"; exit 1; }

cover: coverage ## Open coverage report in browser (run coverage first)
	@echo "Opening coverage report in browser"
	@go tool cover -html=coverage.out

compile: ## Typecheck entire module (catches redeclarations / same errors as gopls DuplicateDecl)
	@echo "Typechecking all packages (go build ./...)"
	@go build ./...

lint: compile ## Run staticcheck after full-module compile
	@echo "Running staticcheck"
	@staticcheck ./...

lint-ci: compile ## go vet + golangci-lint (typecheck + configured linters; see issues.new in .golangci.yml)
	@echo "Running go vet"
	@go vet ./...
	@echo "Running golangci-lint"
	@golangci-lint run ./...

fmt: ## Format Go code with the gofmt of the Go version in go.mod (what CI uses)
	@echo "Running gofmt (go$(GO_MOD_VERSION))"
	@$(GOFMT_CI) -w .

fmt-check: ## Fail if the go.mod-version gofmt or the local gofmt would change any file
	@echo "Checking formatting (gofmt go$(GO_MOD_VERSION) and local $$(go env GOVERSION))"
	@ci="$$($(GOFMT_CI) -l .)"; loc="$$(gofmt -l .)"; \
	if [ -n "$$ci$$loc" ]; then \
		[ -z "$$ci" ] || { echo "Not formatted for gofmt go$(GO_MOD_VERSION) (CI):"; echo "$$ci"; }; \
		[ -z "$$loc" ] || { echo "Not formatted for the local gofmt:"; echo "$$loc"; }; \
		echo "Files must be stable under both. Run 'make fmt'; if the two versions still disagree,"; \
		echo "restructure the code they format differently (e.g. move trailing comments onto their own lines)."; \
		exit 1; \
	fi

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
	@rm -f *.{json,csv,log,mbcap} results/*.{json,csv,log,mbcap}