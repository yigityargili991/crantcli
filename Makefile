BINARY := crantcli
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install clean test

build:
	go build -ldflags "-s -w -X crantcli/cmd.Version=$(VERSION)" -o $(BINARY) .

install:
	go install -ldflags "-X crantcli/cmd.Version=$(VERSION)" .

clean:
	rm -f $(BINARY)

test:
	go test ./...
