# Install

Pre-built binaries are published for macOS, Linux, and Windows on the [GitHub Releases page](https://github.com/yigityargili991/crantcli/releases).

## macOS and Linux

The installer downloads the latest release to `~/.local/bin/crantcli`:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh | sh
```

Make sure `~/.local/bin` is on your `PATH`, then verify the installation:

```bash
crantcli --version
```

Pin a release or choose another destination with environment variables:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh \
  | CRANTCLI_VERSION=vX.Y.Z sh

curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh \
  | CRANTCLI_INSTALL_DIR=/usr/local/bin sh
```

Replace `vX.Y.Z` with a tag from the [Releases page](https://github.com/yigityargili991/crantcli/releases).

### macOS Gatekeeper

If macOS blocks a manually downloaded binary on first launch, clear its quarantine attribute:

```bash
xattr -d com.apple.quarantine "$(which crantcli)"
```

### Linux clipboard support

Clipboard-driven commands need one helper available on the system:

- Wayland: `wl-clipboard` is preferred.
- X11: `xclip` or `xsel`.

For example:

```bash
sudo apt install wl-clipboard
```

You can avoid clipboard dependencies by providing `--state` and `--output` files.

## Windows

Run this in PowerShell:

```powershell
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex
```

The installer selects the correct x64 or ARM64 release, verifies its checksum,
installs it to `%LOCALAPPDATA%\Programs\crantcli`, and adds that directory to
your user `PATH`. The command is available immediately in the same PowerShell
session:

```powershell
crantcli --version
```

Pin a release or choose another destination before running the installer:

```powershell
$env:CRANTCLI_VERSION = "vX.Y.Z"
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex

$env:CRANTCLI_INSTALL_DIR = "C:\Tools\crantcli"
irm https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.ps1 | iex
```

Replace `vX.Y.Z` with a tag from the [Releases page](https://github.com/yigityargili991/crantcli/releases).

## Build from source

Building requires the Go version declared in `go.mod` or newer.

```bash
git clone https://github.com/yigityargili991/crantcli.git
cd crantcli
make build
./crantcli --version
```

Install into your configured Go binary directory:

```bash
make install
```

!!! note "Release asset names"
    The installed command is `crantcli`, while current release assets retain the historical `crant_type_look-<os>-<arch>` filename.
