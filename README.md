# crantcli

[![CI](https://img.shields.io/github/actions/workflow/status/yigityargili991/crantcli/ci.yml?branch=main&style=flat-square&label=CI&logo=github)](https://github.com/yigityargili991/crantcli/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/codecov/c/github/yigityargili991/crantcli?style=flat-square&logo=codecov)](https://codecov.io/gh/yigityargili991/crantcli)
[![Go version](https://img.shields.io/github/go-mod/go-version/yigityargili991/crantcli?style=flat-square&logo=go)](https://github.com/yigityargili991/crantcli/blob/main/go.mod)
[![Release](https://img.shields.io/github/v/release/yigityargili991/crantcli?style=flat-square&logo=github)](https://github.com/yigityargili991/crantcli/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/yigityargili991/crantcli/total?style=flat-square&logo=github)](https://github.com/yigityargili991/crantcli/releases)
[![License](https://img.shields.io/github/license/yigityargili991/crantcli?style=flat-square)](LICENSE)

Query neurons in the [Clonal Raider Ant Connectome](https://github.com/Social-Evolution-and-Behavior/crantpy), add matching root IDs to [Neuroglancer](https://github.com/google/neuroglancer) scenes, and check whether those roots are still current in CAVE.

[Documentation](https://yigityargili991.github.io/crantcli/) · [Installation](docs/getting-started/install.md) · [Guides](docs/guides/query.md) · [Command reference](docs/reference/index.md)

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

Tokens entered through `crantcli setup` are stored in the operating system's
secure credential manager: Keychain on macOS, Credential Manager on Windows,
and Secret Service on Linux. On Linux systems without Secret Service, crantcli
uses a private `0600` file inside an owner-only `~/.crantcli/` directory.
Existing file-based credentials are migrated automatically when possible.

Install a release or build from source first; see the [installation guide](docs/getting-started/install.md).

## What it does

- Explores CRANT classes, types, regions, and counts.
- Builds and colors Neuroglancer scenes from CRANT queries.
- Works with clipboard URLs, JSON files, pipes, or the built-in scene.
- Includes built-in Wayland/X11 clipboard support and XDG portal browser handoff on Linux.
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

Pre-built binaries for Linux, macOS, and Windows on amd64 and arm64 are
available from [GitHub Releases](https://github.com/yigityargili991/crantcli/releases).

### macOS and Linux

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh | sh
```

Pin a release or choose another destination with environment variables:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh \
  | CRANTCLI_VERSION=vX.Y.Z sh

curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh \
  | CRANTCLI_INSTALL_DIR=/usr/local/bin sh
```

### Windows PowerShell

Run this in Windows PowerShell 5.1 or PowerShell 7+; an administrator shell is
not required:

```powershell
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex
crantcli --version
```

The installer selects the x64 or ARM64 binary, verifies its SHA-256 checksum,
installs `crantcli.exe` to
`$env:LOCALAPPDATA\Programs\crantcli`, and adds that directory to both the
current session and your persistent user `PATH`. If `cosign` is installed, it
also verifies the release's keyless Sigstore signature.

Pin a release or choose another destination before running the installer:

```powershell
$env:CRANTCLI_VERSION = "vX.Y.Z"
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex

$env:CRANTCLI_INSTALL_DIR = "C:\Tools\crantcli"
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex
```

Replace `vX.Y.Z` with a published release tag.

Both installers fail closed if release checksums cannot be downloaded or
verified. `CRANTCLI_SKIP_CHECKSUM=1` is available as an explicit insecure
override.

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

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE) for project attribution.
Release assets also include
[third-party notices](THIRD_PARTY_NOTICES.md) for bundled dependencies.
