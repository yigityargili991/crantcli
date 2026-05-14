# crantcli

A CLI tool for the [CRANT](https://github.com/flyconnectome/crant) (Clonal Raider Ant Connectome) dataset. Query neurons by classification, inject root IDs into [Neuroglancer](https://github.com/google/neuroglancer) scenes, check root ID freshness and edit history against [CAVE](https://caveclient.readthedocs.io/), and open visualizations in your browser -- all from the terminal.


## Quick Start

```bash
# Configure your SeaTable and CAVE tokens
crantcli setup

# Query neurons and inject into a Neuroglancer state (reads/writes clipboard)
crantcli add --cell-class kenyon_cell

# Open the result directly in your browser
crantcli add --cell-type ER --open

# Check if a root ID is still current in CAVE
crantcli check-cave 720575940610453042

# Show CAVE edit history for a root ID
crantcli cave-history 720575940610453042
```

## Installation

### From releases

Pre-built binaries for Linux, macOS, and Windows (amd64/arm64) are published on the [Releases](https://github.com/yigityargili991/crantcli/releases) page. Assets are named `crant_type_look-<os>-<arch>` (with `.exe` on Windows).

**macOS / Linux** -- install the latest release to `~/.local/bin/crantcli`:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh | sh
```

To pin a version or choose a different install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh | CRANTCLI_VERSION=v0.10.1 sh
curl -fsSL https://raw.githubusercontent.com/yigityargili991/crantcli/main/install.sh | CRANTCLI_INSTALL_DIR=/usr/local/bin sh
```

Make sure the install directory is on your `PATH`, then verify the install:

```bash
crantcli --version
```

Manual install is also supported. Pick the asset matching `uname -s` (Darwin/Linux) and `uname -m` (arm64/x86_64). Apple Silicon example:

```bash
curl -L -o crantcli https://github.com/yigityargili991/crantcli/releases/latest/download/crant_type_look-darwin-arm64
chmod +x crantcli
mv crantcli ~/.local/bin/    # or another directory on PATH
crantcli --version
```

On macOS, if Gatekeeper blocks the binary on first run, clear the quarantine attribute:

```bash
xattr -d com.apple.quarantine "$(which crantcli)"
```

**Windows** -- download `crant_type_look-windows-amd64.exe` (or `-arm64.exe`) from the Releases page, rename it to `crantcli.exe`, and put it on your `PATH`.

**Linux clipboard helper** -- the clipboard-driven workflows (`add` without `-s`, `state-transfer`, `inspect`) need one of: `wl-clipboard` (Wayland; preferred), `xclip`, or `xsel` (X11). Install via your distro -- e.g. `sudo apt install wl-clipboard` or `sudo apt install xclip`.

### From source

Requires Go 1.25.5+.

```bash
git clone https://github.com/yigityargili991/crantcli.git
cd crantcli
make build      # produces ./crantcli
make install    # installs to $GOBIN (see `go env GOBIN`, defaults to $GOPATH/bin)
```

## Commands

### `add` -- Query and inject neurons

The main command. Queries the CRANT dataset and injects matching root IDs into a Neuroglancer state.

```bash
# Smart mode: reads state from clipboard, injects, copies back
crantcli add --cell-class kenyon_cell

# Explicit file I/O
crantcli add --cell-class kenyon_cell -s state.json -o modified.json

# Start from the default CRANT scene template
crantcli add --cell-class kenyon_cell --generate

# Combine filters
crantcli add --super-class sensory --side left --color "#ff0000"

# Color matched neurons by a metadata field
crantcli add --super-class sensory --color-by cell_type

# Use a named palette across color-by groups
crantcli add --region LX --color-by side --color blue

# Query by bundle/region annotation
crantcli add --bundle LX

# Replace segments instead of appending
crantcli add --cell-type ER --replace

# Just get root IDs, no state manipulation
crantcli add --cell-class kenyon_cell --root-ids-only
```

**Filter flags:** `--super-class`, `--cell-class`, `--cell-type`, `--cell-subtype`, `--side`, `--region`, `--bundle`, `--tract`, `--proofread`

**Color flags:** `--color` (named palette, `colored`, or 6-digit hex), `--color-by` (group colors by `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, or `proofread`), `--color-sub` (sub-color by `cell_subtype` within each query group). When `--color-by` is supplied without `--color`, it defaults to palette cycling (`colored`).

**State flags:** `-s`/`--state` (URL or file), `-g`/`--generate` (use default template), `-o`/`--output` (file path), `-l`/`--layer` (target layer name), `--replace`, `--open`

**Smart input resolution** (when no `--state` is given):
1. stdin (piped JSON)
2. Clipboard (Neuroglancer URL)
3. Last state URL from a previous session
4. Default CRANT scene template

### `check-cave` -- Verify root ID freshness

Check whether root IDs stored in SeaTable still match the current CAVE chunkedgraph. Supervoxel IDs are stable, but root IDs change when proofreading edits (merges/splits) happen. This command detects stale entries.

```bash
# Check a single root ID
crantcli check-cave 720575940610453042

# Check multiple root IDs
crantcli check-cave 720575940610453042 720575940631928371

# Check all neurons in the table
crantcli check-cave --all

# Check a filtered subset
crantcli check-cave --all --cell-class kenyon_cell

# Only print stale entries (exit code 1 if any found)
crantcli check-cave --all --quiet

# Print stale mappings as old_root_id<TAB>current_cave_root_id
crantcli check-cave --all --mapping

# Replace stale segment IDs in a Neuroglancer state
crantcli check-cave --all --refresh-state -s state.json -o refreshed.json

# Refresh only the selected segmentation layer
crantcli check-cave --all --refresh-state -s state.json -o refreshed.json -l "proofreadable seg"
```

**Filter flags:** Same as `add` (`--super-class`, `--cell-class`, `--cell-type`, etc.)

**Flags:** `--all` (check all neurons), `-q`/`--quiet` (only print stale entries), `--mapping` (print stale `old_root_id<TAB>current_cave_root_id` pairs), `--refresh-state` (replace stale segment IDs in a Neuroglancer state), `-s`/`--state` (state URL or file for refresh), `-o`/`--output` (refreshed state file), `-l`/`--layer` (target segmentation layer)

`--refresh-state` uses the same smart state loading and output behavior as the state-editing commands: with `-s` it reads a URL or JSON file, and with `-o` it writes JSON to a file. Without `-o`, output follows the loaded state source (clipboard/URL sources are written back as a Neuroglancer URL; file/stdin sources are written as JSON to stdout). Refresh mode writes the updated state instead of the normal check table. If you combine `--mapping` and `--refresh-state`, provide `-o` so mapping output and state JSON do not share stdout.

Requires a CAVE token (configured via `crantcli setup` or the `CAVE_TOKEN` / `CAVE_TOKEN_FILE` environment variables).

### `cave-history` -- Show CAVE edit history

Show tabular CAVE changelog rows for one or more root IDs. By default, history is filtered to edits that affect the final state of the queried root.

```bash
# Show readable history table
crantcli cave-history 720575940610453042

# Check multiple roots
crantcli cave-history 720575940610453042 720575940631928371

# Print stable JSON output
crantcli cave-history 720575940610453042 --json

# Include broader split/merge history
crantcli cave-history 720575940610453042 --unfiltered
```

**Flags:** `--json` (print JSON result objects), `--unfiltered` (request unfiltered CAVE history)

Requires a CAVE token (configured via `crantcli setup` or the `CAVE_TOKEN` / `CAVE_TOKEN_FILE` environment variables).

### `list` -- Explore the dataset

List distinct values for any classification field, optionally with neuron counts. `region` values are printed as resolved region names rather than raw SeaTable option IDs.

```bash
crantcli list super_class --count
crantcli list cell_type --cell-class kenyon_cell
crantcli list cell_class --super-class sensory --count
```

Valid fields: `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread`

### `inspect` -- View state structure

Display layers, segment counts, and color assignments in a Neuroglancer state.

```bash
crantcli inspect                # reads from clipboard
crantcli inspect -s state.json
```

### `lookup-column` -- Find the closest EPG/PEG column

Finds the closest EPG/PEG neuron to a given root ID (or position) by 3D Euclidean distance and prints its resolved `region` value plus the nearest EPG/PEG root ID.

```bash
# Look up by root ID
crantcli lookup-column 720575940610453042

# Provide position directly
crantcli lookup-column --pos 31870.5,26635.5,1502.5
```

**Flags:** `--pos` (comma-separated `x,y,z` coordinates; skips the root ID lookup)

### `side-check` -- Check side annotations against nearest EPG/PEG

Checks neurons selected by exactly one classifier against the nearest valid `EPG/PEG` neuron by 3D Euclidean distance. Prints one selected `root_id` per line when side is missing, position is missing or malformed, or side matches the nearest valid `EPG/PEG`.

```bash
crantcli side-check --cell-type PFN
crantcli side-check --cell-class some_class
```

**Flags:** `--cell-type`, `--cell-class` (provide exactly one)

### `state-transfer` -- Build state from clipboard IDs

Read root IDs from the clipboard, inject them into a Neuroglancer state, and copy the resulting state URL back to the clipboard. Useful when you have a list of root IDs from another source and want to quickly visualize them.

```bash
# Copy some root IDs to clipboard, then:
crantcli state-transfer

# Use a specific base state
crantcli state-transfer -s base.json

# Target a specific segmentation layer
crantcli state-transfer -l "my layer"

# Write to file instead of clipboard
crantcli state-transfer -o output.json
```

IDs in the clipboard can be separated by whitespace, newlines, or commas. Duplicates are removed automatically.

**Flags:** `-s`/`--state` (base state URL or file), `-o`/`--output` (file path), `-l`/`--layer` (target layer name), `--color` (segment color)

### `generate` -- Output default template

Print the built-in CRANT scene template to stdout.

```bash
crantcli generate > my_scene.json
```

### `change-def-state` -- Set the default Neuroglancer state

Set or update the default Neuroglancer JSON state used when no other state source is available.

```bash
crantcli change-def-state /path/to/state.json
crantcli change-def-state --show
crantcli change-def-state --reset
```

**Flags:** `--show` (display current default state), `--reset` (reset to built-in template)

### `setup` -- Configure credentials

Interactively set your SeaTable API token and optional CAVE token. Stored in `~/.crantcli/`.

```bash
crantcli setup
```

Tokens can also be provided via environment variables:
- **SeaTable:** `CRANTTABLE_TOKEN` or `CRANTTABLE_TOKEN_FILE`
- **CAVE:** `CAVE_TOKEN` or `CAVE_TOKEN_FILE`

## Testing

```bash
make test
# or
go test ./...
```

## License

This project is licensed under the [MIT License](LICENSE).
