# Install

Pre-built binaries are published for macOS, Linux, and Windows on the [GitHub Releases page](https://github.com/yigityargili991/crantcli/releases).

!!! warning "Pre-launch repository access"
    This repository is currently private. Anonymous release and raw-file URLs
    return 404 until public launch. Collaborators can download a release while
    signed in to GitHub, or build from an authenticated clone.

## macOS and Linux

Once the repository is public, the installer downloads the latest release to
`~/.local/bin/crantcli`:

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

1. Download `crant_type_look-windows-amd64.exe` or `crant_type_look-windows-arm64.exe` from [Releases](https://github.com/yigityargili991/crantcli/releases).
2. Rename it to `crantcli.exe`.
3. Move it into a directory on your `PATH`.
4. Run `crantcli --version` in PowerShell.

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
