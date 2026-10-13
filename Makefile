BINARY := crantcli
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# The ask model runtime needs SSE (x86-64-v2) for its SIMD kernels; the
# baseline build runs the model about 50 times slower. arm64 ignores this.
export GOAMD64 ?= v2

.PHONY: build install clean test docs-generate docs-serve docs-build docs-check

build:
	go build -ldflags "-s -w -X crantcli/cmd.Version=$(VERSION)" -o $(BINARY) .

install:
	go install -ldflags "-X crantcli/cmd.Version=$(VERSION)" .

clean:
	rm -f $(BINARY)

test:
	go test ./...

docs-generate:
	go run ./tools/docgen

docs-serve: docs-generate
	mkdocs serve

docs-build: docs-generate
	mkdocs build --strict

docs-check: docs-generate
	git diff --exit-code -- docs/reference/commands
	test -z "$$(git ls-files --others --exclude-standard docs/reference/commands)"
	mkdocs build --strict
