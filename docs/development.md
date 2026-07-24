# Development

## Build and test

The project uses the Go version declared in `go.mod`.

```bash
make build
make test
```

The same checks can be run directly:

```bash
go vet ./...
go test ./...
```

## Work on the documentation

Create a Python environment and install the pinned documentation dependencies:

```bash
python3 -m venv .venv-docs
source .venv-docs/bin/activate
python -m pip install -r requirements-docs.txt
```

To update the documentation toolchain, edit `requirements-docs.in` and rebuild
the cross-platform hash lock:

```bash
uv pip compile requirements-docs.in --generate-hashes --output-file requirements-docs.txt
```

Generate command pages and start a live preview:

```bash
make docs-serve
```

Build with warnings treated as errors:

```bash
make docs-build
```

## Keep command reference current

Command pages under `docs/reference/commands/` are generated from Cobra:

```bash
make docs-generate
```

Edit command descriptions and flag help in `cmd/`, then regenerate. Do not hand-edit generated pages.

Check both freshness and the site build:

```bash
make docs-check
```

## Documentation structure

```text
docs/
├── getting-started/   first-use path
├── guides/            task-oriented workflows
├── concepts/          mental models and semantics
├── reference/         generated command reference
└── help/              troubleshooting and completion
```

The repository README stays concise and points to the site as the canonical manual.

## Release artifacts

Tagged releases build binaries for Linux, macOS, and Windows on amd64 and arm64. The binary is called `crantcli`; release files currently use the historical `crant_type_look-<os>-<arch>` prefix.
