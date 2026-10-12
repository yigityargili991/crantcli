# Development

## Build and test

The project uses the Go version declared in `go.mod`.

```bash
make build
make test
```

CI runs `go vet ./...` and `go test ./...`.

## Build the experimental ask command

`crantcli ask` turns a plain-English request into a crantcli command with a
small local model. It is only built with the `ask` tag, because the model
runtime adds about 30 MB to the binary:

```bash
make build-ask
CRANTCLI_ASK_MODEL=/path/to/model.gguf ./crantcli ask "show the PEN neurons colored by proofreading status"
```

`ask` prints the suggested command and asks before running it. `--print` only
prints it, and `--yes` runs it without asking. The model is read from
`--model`, then `CRANTCLI_ASK_MODEL`, then `<user cache dir>/crantcli/ask/model.gguf`.
Table values are cached next to it for a day; `--refresh` fetches them again.

The command pages under `docs/reference/commands/` are generated from the
default build, so they do not list `ask`.

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

The release workflow creates the GitHub release as a draft, uploads every binary, signature, installer, and `checksums.txt`, and only then publishes it. Until that final step, `releases/latest` still resolves to the previous release, so `install.sh` and `install.ps1` never see a release whose assets are missing or still uploading.
