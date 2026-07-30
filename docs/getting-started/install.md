# Install

Pre-built binaries are published for macOS, Linux, and Windows on the [GitHub Releases page](https://github.com/yigityargili991/crantcli/releases).

Linux binaries are statically linked and support Debian, Ubuntu, Arch Linux, and
compatible distributions on amd64 and arm64.

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

### Linux desktop integration

Clipboard access is built into the Linux binary for Wayland and X11; no
clipboard package is required. If installed, `wl-copy`/`wl-paste`, `xclip`, or
`xsel` remain compatibility fallbacks for unusual compositor setups.

Browser opening uses the XDG desktop portal when available, then falls back to
`xdg-open` or `gio`. Headless sessions can use explicit files instead of desktop
integration.

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
