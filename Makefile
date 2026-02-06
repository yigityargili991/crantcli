BINARY := crant_type_look
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install clean test

build:
	go build -ldflags "-s -w" -o $(BINARY) .

install:
	go install .

clean:
	rm -f $(BINARY)

test:
	go test ./...
