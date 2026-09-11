# TiBrain Makefile
# Build, test, and manage TiBrain lifecycle

# Variables
APP_NAME    := tibrain
MODULE      := github.com/ti/router/tibrain
BUILD_DIR   := build
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go flags
GO          := go
GOFLAGS     := -trimpath
LDFLAGS     := -s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildTime=$(BUILD_TIME) \
	-X main.GitCommit=$(GIT_COMMIT)

# OS/Arch detection
OS   := $(shell go env GOOS)
ARCH := $(shell go env GOARCH)

# Binary name
ifeq ($(OS),windows)
	BINARY := $(APP_NAME).exe
else
	BINARY := $(APP_NAME)
endif

# Default target
.PHONY: all
all: build

# ─────────────────────────────────────────────────────────────
# Build Targets
# ─────────────────────────────────────────────────────────────

# Build with optimizations
.PHONY: build
build:
	@echo "🔨 Building $(APP_NAME) $(VERSION) for $(OS)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) .
	@echo "✅ Built $(BUILD_DIR)/$(BINARY)"
	@ls -lh $(BUILD_DIR)/$(BINARY)

# Build without optimizations (for debugging)
.PHONY: build-debug
build-debug:
	@echo "🔨 Building $(APP_NAME) (debug) for $(OS)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY) .
	@echo "✅ Built $(BUILD_DIR)/$(BINARY)"

# Build for multiple platforms
.PHONY: build-all
build-all: build-linux build-windows build-darwin

.PHONY: build-linux
build-linux:
	@echo "🔨 Building for linux/amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .

.PHONY: build-windows
build-windows:
	@echo "🔨 Building for windows/amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe .

.PHONY: build-darwin
build-darwin:
	@echo "🔨 Building for darwin/arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .

# ─────────────────────────────────────────────────────────────
# Test Targets
# ─────────────────────────────────────────────────────────────

.PHONY: test
test:
	@echo "🧪 Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	@echo "📊 Coverage report:"
	$(GO) tool cover -func=coverage.out

.PHONY: test-unit
test-unit:
	@echo "🧪 Running unit tests..."
	$(GO) test -v -race ./...

.PHONY: test-integration
test-integration:
	@echo "🧪 Running integration tests..."
	$(GO) test -v -race -tags=integration ./...

.PHONY: test-short
test-short: ## Skip slow tests (fast feedback)
	@echo "🧪 Running short tests..."
	$(GO) test -short ./...

.PHONY: test-coverage-html
test-coverage-html: ## Generate HTML coverage report
	@echo "📊 Generating HTML coverage report..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "✅ Open coverage.html in your browser"

# ─────────────────────────────────────────────────────────────
# Lint & Quality
# ─────────────────────────────────────────────────────────────

.PHONY: lint
lint:
	@echo "🔍 Running linter..."
	golangci-lint run ./...

.PHONY: fmt
fmt:
	@echo "📝 Formatting code..."
	$(GO) fmt ./...

.PHONY: vet
vet:
	@echo "🔍 Running go vet..."
	$(GO) vet ./...

.PHONY: tidy
tidy:
	@echo "📦 Tidying modules..."
	$(GO) mod tidy

# ─────────────────────────────────────────────────────────────
# Run Targets
# ─────────────────────────────────────────────────────────────

.PHONY: run
run: build
	@echo "🚀 Starting $(APP_NAME)..."
	./$(BUILD_DIR)/$(BINARY)

.PHONY: run-debug
run-debug: build-debug
	@echo "🚀 Starting $(APP_NAME) (debug)..."
	./$(BUILD_DIR)/$(BINARY)

.PHONY: run-index
run-index: build
	@echo "📚 Running knowledge indexing..."
	./$(BUILD_DIR)/$(BINARY) --index-knowledge

# ─────────────────────────────────────────────────────────────
# Tunnel Targets
# ─────────────────────────────────────────────────────────────

CLOUDFLARED_CONFIG ?= Z:\02_CORE\_cli\.config\.cloudflared\config.yml

.PHONY: tunnel
tunnel: ## Start Cloudflare tunnel for TiBrain
	@echo "🚀 Starting Cloudflare tunnel for TiBrain..."
	@cloudflared tunnel --config "$(CLOUDFLARED_CONFIG)" run

.PHONY: tunnel-start
tunnel-start: ## Start tunnel in background (Windows PowerShell)
	@echo "🚀 Starting tunnel in background..."
	@powershell -Command "Start-Process -FilePath 'cloudflared.exe' -ArgumentList 'tunnel','--config','$(CLOUDFLARED_CONFIG)','run' -WindowStyle Hidden"

.PHONY: start
start: run tunnel-start ## Start TiBrain + Cloudflare tunnel

# ─────────────────────────────────────────────────────────────
# Composite Targets
# ─────────────────────────────────────────────────────────────

.PHONY: verify
verify: vet lint test-short ## Pre-commit gate (vet + lint + fast tests)
	@echo "✅ All pre-commit checks passed"

.PHONY: bench
bench: ## Run benchmarks
	@echo "📊 Running benchmarks..."
	$(GO) test -bench=. -benchmem ./...

.PHONY: docker
docker: ## Build Docker image
	@echo "🐳 Building Docker image..."
	docker build -t $(APP_NAME):$(VERSION) .

# ─────────────────────────────────────────────────────────────
# Clean
# ─────────────────────────────────────────────────────────────

.PHONY: clean
clean:
	@echo "🧹 Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out
	rm -f *.exe
	rm -f *.test
	@echo "✅ Cleaned"

# ─────────────────────────────────────────────────────────────
# Info
# ─────────────────────────────────────────────────────────────

.PHONY: info
info:
	@echo "📋 TiBrain Build Info:"
	@echo "  Version:    $(VERSION)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo "  OS/Arch:    $(OS)/$(ARCH)"
	@echo "  Binary:     $(BINARY)"
	@echo "  Module:     $(MODULE)"
	@$(GO) version

# ─────────────────────────────────────────────────────────────
# Help
# ─────────────────────────────────────────────────────────────

.PHONY: help
help:
	@echo "TiBrain Makefile - Available targets:"
	@echo ""
	@echo "  build          Build with optimizations (default)"
	@echo "  build-debug    Build without optimizations (for debugging)"
	@echo "  build-all      Build for linux, windows, darwin"
	@echo "  test           Run all tests with coverage"
	@echo "  test-unit      Run unit tests only"
	@echo "  test-integration Run integration tests"
	@echo "  test-short     Skip slow tests (fast feedback)"
	@echo "  test-coverage-html Generate HTML coverage report"
	@echo "  lint           Run golangci-lint"
	@echo "  fmt            Format code with gofmt"
	@echo "  vet            Run go vet"
	@echo "  tidy           Run go mod tidy"
	@echo "  run            Build and run TiBrain server"
	@echo "  run-debug      Build debug and run"
	@echo "  run-index      Build and run knowledge indexing"
	@echo "  tunnel         Start Cloudflare tunnel (blocks)"
	@echo "  tunnel-start   Start Cloudflare tunnel in background"
	@echo "  start          Build and run TiBrain + Cloudflare tunnel together"
	@echo "  sync-omniroute Sync OmniRoute knowledge into TiBrain"
	@echo "  verify         Pre-commit gate (vet + lint + test-short)"
	@echo "  bench          Run benchmarks"
	@echo "  docker         Build Docker image"
	@echo "  clean          Remove build artifacts"
	@echo "  info           Show build information"
	@echo "  help           Show this help message"

.PHONY: sync-omniroute
sync-omniroute:
	py scripts/sync_omniroute_knowledge.py
