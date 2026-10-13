# Development

## Build and test

The project uses the Go version declared in `go.mod`.

```bash
make build
make test
```

CI runs `go vet ./...` and `go test ./...`.

## Publish a raider model

`crantcli ask` downloads the raider model from a GitHub release. The model is
trained in the private `raider-training` repository. To publish a new version
`vN`:

1. Encrypt the GGUF with a SeaTable token for the CRANT base configured:

   ```bash
   go run ./tools/modelenc -in raider-135m-vN-q8_0.gguf -version vN
   ```

   This writes `raider-135m-vN-q8_0.gguf.enc` and prints the size, hashes, and
   key check to paste into `CurrentModel` in `internal/ask/model.go`, together
   with the new `Version`, `File`, `Tag`, and `Asset` names.

2. Publish only the encrypted file, as a prerelease so that `crantcli update`
   keeps using the latest crantcli release:

   ```bash
   gh release create raider-vN raider-135m-vN-q8_0.gguf.enc --prerelease --latest=false \
     --title "raider vN model" --notes "Encrypted model for crantcli ask."
   ```

3. Release crantcli with the new pin. Never upload the decrypted GGUF.

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
