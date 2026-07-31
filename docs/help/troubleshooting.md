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

## Clipboard or browser delivery fails on Linux

The release binary has built-in Wayland and X11 clipboard support. It also uses
`wl-copy`/`wl-paste`, `xclip`, or `xsel` as compatibility fallbacks when they
are already installed. A failure normally means the process has no usable
graphical session—for example, a headless shell with neither
`WAYLAND_DISPLAY` nor `DISPLAY`.

Delivery failures are explicit but not fatal on their own. If clipboard delivery
fails for a URL-producing command, crantcli warns and prints the completed URL
to standard output, and the command still succeeds. Only a result that reached
no destination at all is reported as a failure.

File-based workflows avoid desktop integration entirely:

```bash
crantcli add --cell-type ER --state input.json --output result.json
```

On Linux, `--open` first uses the XDG desktop portal and then `xdg-open` or
`gio`. The desktop may open a tab in an existing browser window without moving
that window between workspaces; crantcli can verify the handoff, but the window
manager controls focus.

Every platform caps the length of a command-line argument, and Neuroglancer
states routinely exceed the tightest of those caps (32767 characters on
Windows). When a URL is too long to pass as an argument, crantcli stages it in a
private redirect file under the user cache directory and opens that instead; the
browser lands on the real viewer URL. Staged files are readable only by you and
are swept after 24 hours.

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
