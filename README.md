# crantcli

A CLI tool for the [CRANT](https://github.com/flyconnectome/crant) (Connectome Reconstruction and Analysis of Neural Tissue) ant connectome dataset. Query neurons by classification, inject root IDs into [Neuroglancer](https://github.com/google/neuroglancer) scenes, check root ID freshness against [CAVE](https://caveclient.readthedocs.io/), and open visualizations in your browser -- all from the terminal.


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
```

## Installation

### From source

Requires Go 1.25.5+.

```bash
git clone https://github.com/yigityargili991/crantinject.git
cd crantinject
make build      # produces ./crantcli
make install    # installs to $GOBIN (see `go env GOBIN`, defaults to $GOPATH/bin)
```

### From releases

Pre-built binaries for Linux, macOS, and Windows (amd64/arm64) are available on the [Releases](https://github.com/yigityargili991/crantinject/releases) page.

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

# Query by bundle/region annotation
crantcli add --bundle LX

# Replace segments instead of appending
crantcli add --cell-type ER --replace

# Just get root IDs, no state manipulation
crantcli add --cell-class kenyon_cell --root-ids-only
```

**Filter flags:** `--super-class`, `--cell-class`, `--cell-type`, `--cell-subtype`, `--side`, `--region`, `--bundle`, `--tract`, `--proofread`

**State flags:** `-s`/`--state` (URL or file), `-g`/`--generate` (use default template), `-o`/`--output` (file path), `-l`/`--layer` (target layer name), `--color` (named palette, `colored`, or 6-digit hex), `--replace`, `--open`

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
```

**Filter flags:** Same as `add` (`--super-class`, `--cell-class`, `--cell-type`, etc.)

**Flags:** `--all` (check all neurons), `-q`/`--quiet` (only print stale entries)

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
