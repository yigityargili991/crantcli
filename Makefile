BUILD_DIR := dist
BINARY := $(BUILD_DIR)/crantcli
VIEWER_BINARY := $(BUILD_DIR)/crantcli-skeleton-viewer
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build build-cli build-viewer build-viewer-headless install install-cli install-viewer clean test

build: build-cli build-viewer

build-cli:
	mkdir -p $(BUILD_DIR)
	go build -ldflags "-s -w -X crantcli/cmd.Version=$(VERSION)" -o $(BINARY) .

build-viewer:
	mkdir -p $(BUILD_DIR)
	go build -ldflags "-s -w" -o $(VIEWER_BINARY) ./cmd/crantcli-skeleton-viewer

build-viewer-headless:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -tags headless -ldflags "-s -w" -o $(VIEWER_BINARY)-headless ./cmd/crantcli-skeleton-viewer

install: install-cli install-viewer

install-cli:
	go install -ldflags "-X crantcli/cmd.Version=$(VERSION)" .

install-viewer:
	go install ./cmd/crantcli-skeleton-viewer

clean:
	rm -f $(BINARY) $(VIEWER_BINARY) $(VIEWER_BINARY)-headless

test:
	go test ./...
