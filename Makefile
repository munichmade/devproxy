.PHONY: build build-all clean test test-coverage lint fmt install uninstall release release-snapshot help

# Version and build info
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X github.com/munichmade/devproxy/cmd/devproxy/cmd.Version=$(VERSION) -X github.com/munichmade/devproxy/cmd/devproxy/cmd.Commit=$(COMMIT) -X github.com/munichmade/devproxy/cmd/devproxy/cmd.BuildDate=$(BUILD_DATE)"

# Binary info
BINARY := devproxy
BUILD_DIR := bin
INSTALL_DIR := /usr/local/bin

# Platforms for cross-compilation
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

# Default target
.DEFAULT_GOAL := build

## help: Show this help message
help:
	@echo "DevProxy - Local Development Reverse Proxy"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: Build binary for current platform
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/devproxy

## build-all: Build for all platforms (darwin/linux, amd64/arm64)
build-all:
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-$${platform%/*}-$${platform#*/} ./cmd/devproxy; \
		echo "Built $(BINARY)-$${platform%/*}-$${platform#*/}"; \
	done

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean -cache -testcache

## test: Run tests
test:
	go test -v ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@mkdir -p $(BUILD_DIR)
	go test -v -coverprofile=$(BUILD_DIR)/coverage.out ./...
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"

## lint: Run golangci-lint
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

## fmt: Format code
fmt:
	go fmt ./...
	@which goimports > /dev/null && goimports -local github.com/munichmade/devproxy -w . || true

## install: Build and install to /usr/local/bin
install: build
	@echo "Installing $(BINARY) to $(INSTALL_DIR)..."
	@sudo rm $(INSTALL_DIR)/$(BINARY)
	@sudo cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@sudo chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "Installed successfully"

## uninstall: Remove from /usr/local/bin
uninstall:
	@echo "Removing $(BINARY) from $(INSTALL_DIR)..."
	@sudo rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "Uninstalled successfully"

## release: Create release archives for all platforms
release: build-all
	@mkdir -p $(BUILD_DIR)/release
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		name=$(BINARY)-$(VERSION)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then \
			zip -j $(BUILD_DIR)/release/$$name.zip $(BUILD_DIR)/$(BINARY)-$$os-$$arch; \
		else \
			tar -czf $(BUILD_DIR)/release/$$name.tar.gz -C $(BUILD_DIR) $(BINARY)-$$os-$$arch; \
		fi; \
		echo "Created $$name archive"; \
	done
	@echo "Release archives in $(BUILD_DIR)/release/"

## run: Run the daemon in foreground
run: build
	$(BUILD_DIR)/$(BINARY) run

## dev: Run with live reload (requires entr)
dev:
	@which entr > /dev/null || (echo "Please install entr: brew install entr" && exit 1)
	find . -name '*.go' | entr -r make run

## release-snapshot: Test release locally without publishing (uses goreleaser)
release-snapshot:
	@which goreleaser > /dev/null || (echo "Installing goreleaser..." && go install github.com/goreleaser/goreleaser/v2@latest)
	goreleaser release --snapshot --clean
