.PHONY: build clean test install

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY := devproxy
BUILD_DIR := bin

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/devproxy

clean:
	rm -rf $(BUILD_DIR)

test:
	go test -v ./...

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
