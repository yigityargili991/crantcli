# Troubleshooting

## `crantcli: command not found`

Confirm the install directory is on `PATH`:

```bash
command -v crantcli
echo "$PATH"
```

The install script uses `~/.local/bin` by default. Add it to your shell profile if needed:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## macOS blocks the binary

For a manually downloaded release:

```bash
xattr -d com.apple.quarantine "$(which crantcli)"
```

## No SeaTable token is configured

Run interactive setup:

```bash
crantcli setup
```

For a non-interactive shell:

```bash
export CRANTTABLE_TOKEN="..."
```

Credentials written by `setup` take precedence over `CRANTTABLE_TOKEN`.

## The system credential store is unavailable

On macOS, unlock Keychain. On Windows, confirm Credential Manager is available.
On Linux desktops, unlock or start a Secret Service provider such as GNOME
Keyring or KWallet.

Headless Linux systems automatically use an owner-only credential file. macOS
and Windows fail closed instead; use a mounted secret file when interactive
credential storage is unavailable:

```bash
export CRANTTABLE_TOKEN_FILE="/run/secrets/cranttable"
export CAVE_TOKEN_FILE="/run/secrets/cave"
```

## CAVE commands report a missing token

Run `crantcli setup` again and enter the optional CAVE token, or set:

```bash
export CAVE_TOKEN="..."
```

`check-cave` and `root-info` also query SeaTable, so they require both services.

## Clipboard reading or writing fails on Linux

Install a compatible helper:

```bash
# Wayland
sudo apt install wl-clipboard

# X11
sudo apt install xclip   # or xsel
```

The helper is used only when `WAYLAND_DISPLAY` (Wayland) or `DISPLAY` (X11) is set.

File-based workflows avoid the clipboard:

```bash
crantcli add --cell-type ER --state input.json --output result.json
```

## The wrong segmentation layer was changed

Without `--layer`, `crantcli` uses the first segmentation layer. Inspect the state and choose explicitly:

```bash
crantcli inspect --state scene.json
crantcli add --cell-type ER --state scene.json --layer "proofreadable seg — SP inputs colored"
```

## `--labels` cannot find or authenticate `gh`

Install the [GitHub CLI](https://cli.github.com/), then authenticate:

```bash
gh auth login
gh auth status
```

Alternatively, provide a [custom label publisher](../guides/labels.md#use-a-custom-publisher).

## Labels disappeared from a saved state

The hosted label source was probably cleaned up. Cleaning a gist or custom source leaves segment IDs intact but makes referenced labels unavailable. Re-run the query with `--labels` to publish and attach a new source.

## `check-cave --quiet` exits with status 1

This is expected when stale root IDs are found. Quiet mode is designed for scripts:

```bash
if crantcli check-cave --all --quiet; then
  echo "All checked roots are current"
else
  echo "At least one checked root is stale"
fi
```

## A piped command ignores the clipboard or `--generate`

Non-empty piped JSON has the highest priority when `--state` is absent. Remove the pipe or provide `--state` explicitly.
