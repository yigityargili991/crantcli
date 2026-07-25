# crantcli

Query neurons in the [Clonal Raider Ant Connectome](https://github.com/flyconnectome/crant), add matching root IDs to [Neuroglancer](https://github.com/google/neuroglancer) scenes, and check whether those roots are still current in CAVE.

[Documentation](docs/index.md) · [Installation](docs/getting-started/install.md) · [Guides](docs/guides/query.md) · [Command reference](docs/reference/index.md)

## Quick start

```bash
# Configure SeaTable and CAVE access
crantcli setup

# Query a population, color it by type, and open Neuroglancer
crantcli add \
  --cell-type ER \
  --color-by cell_type \
  --generate \
  --open
```

Tokens are stored under `~/.crantcli/` with 0600 permissions as base64-encoded
files. Base64 is obfuscation, not encryption — the file permissions are what
protect the tokens, and crantcli re-tightens them if they are ever loosened.

Install a release or build from source first; see the [installation guide](docs/getting-started/install.md).

## What it does

- Explores CRANT classes, types, regions, and counts.
- Builds and colors Neuroglancer scenes from CRANT queries.
- Works with clipboard URLs, JSON files, pipes, or the built-in scene.
- Checks stored root IDs against CAVE and refreshes stale scene segments.
- Shows CAVE edit history and combined root metadata.
- Adds CRANT cell-type labels to Neuroglancer segments.
- Generates completion for Bash, Zsh, Fish, and PowerShell.

## A few useful commands

```bash
# Explore available values
crantcli list cell_type --cell-class ER --count

# Add two populations as independently colored groups
crantcli add --cell-type ER --cell-type EPG/PEG --color colored

# Work with explicit files instead of the clipboard
crantcli add --cell-type ER --state base.json --output result.json

# Check and refresh stale root IDs
crantcli check-cave --all
crantcli check-cave --all --refresh-state --state scene.json --output refreshed.json

# Inspect everything known about one root
crantcli root-info 720575940610453042
```

The [user guide](https://yigityargili991.github.io/crantcli/) explains query grouping, coloring, state resolution, CAVE freshness, labels, and troubleshooting. Flag-level pages are generated from the Cobra command tree.

## Install

Pre-built binaries for Linux, macOS, and Windows on amd64 and arm64 are available from [GitHub Releases](https://github.com/yigityargili991/crantcli/releases).

macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex
```

The installer supports pinned versions and custom destinations:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh \
  | CRANTCLI_VERSION=vX.Y.Z sh

curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh \
  | CRANTCLI_INSTALL_DIR=/usr/local/bin sh
```

In PowerShell, set the equivalent variables before running the one-liner:

```powershell
$env:CRANTCLI_VERSION = "vX.Y.Z"
$env:CRANTCLI_INSTALL_DIR = "C:\Tools\crantcli"
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex
```

Replace `vX.Y.Z` with a published release tag.

The installer fails closed: if the release checksums cannot be downloaded or no
SHA-256 tool is available, it refuses to install (set `CRANTCLI_SKIP_CHECKSUM=1`
to override). Releases are also signed keylessly with cosign; if `cosign` is
installed, the installer verifies the signature automatically.

To build from source:

```bash
git clone https://github.com/yigityargili991/crantcli.git
cd crantcli
make build
```

## Develop

```bash
make test
make build
```

Documentation contributors can install `requirements-docs.txt`, then run:

```bash
make docs-serve
make docs-check
```

See the [development guide](docs/development.md) for the documentation workflow.

## License

[MIT](LICENSE)
