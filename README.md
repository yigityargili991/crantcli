# crantinject

A CLI tool for querying the [CRANT](https://github.com/flyconnectome/crant) (Connectome Reconstruction and Analysis of Neural Tissue) dataset and injecting neuron root IDs into [Neuroglancer](https://github.com/google/neuroglancer) visualization states.

Query neurons by classification, get their root IDs into a Neuroglancer scene, and open it in your browser -- all in one command.

## Quick Start

```bash
# Configure your SeaTable API token
crantinject setup

# Query neurons and inject into a Neuroglancer state (reads/writes clipboard)
crantinject add --cell-class kenyon_cell

# Open the result directly in your browser
crantinject add --cell-type ER --open
```

## Installation

### From source

Requires Go 1.25.5+.

```bash
git clone https://github.com/yigityargili991/crantinject.git
cd crantinject
make build      # produces ./crantinject
make install    # installs to $GOBIN (see `go env GOBIN`, defaults to $GOPATH/bin)
```

### From releases

Pre-built binaries for Linux, macOS, and Windows (amd64/arm64) are available on the [Releases](https://github.com/yigityargili991/crantinject/releases) page.

## Commands

### `add` -- Query and inject neurons

The main command. Queries the CRANT dataset and injects matching root IDs into a Neuroglancer state.

```bash
# Default mode: reads state from clipboard, injects, copies back
crantinject add --cell-class kenyon_cell

# Explicit file I/O
crantinject add --cell-class kenyon_cell -s state.json -o modified.json

# Start from the default CRANT scene template
crantinject add --cell-class kenyon_cell --generate

# Combine filters
crantinject add --super-class sensory --side left --color "#ff0000"

# Reset to a clean template, then add selected neurons
crantinject add --cell-type ER --unpile

# Just get root IDs, no state manipulation
crantinject add --cell-class kenyon_cell --root-ids-only
```

**Filter flags:** `--super-class`, `--cell-class`, `--cell-type`, `--cell-subtype`, `--side`, `--region`, `--tract`, `--proofread`

**State flags:** `-s`/`--state` (URL or file), `-g`/`--generate` (use default template), `-o`/`--output` (file path), `-l`/`--layer` (target layer name), `--color` (hex color), `--unpile` (reset to clean template before adding), `--open`

**Default input behavior** (when no `--state`, `--generate`, or `--unpile` is given):
1. Clipboard (must contain a Neuroglancer URL)

If clipboard does not contain a valid Neuroglancer URL, `add` returns an error. Use `--unpile` to start clean or `--state`/`--generate` for explicit input.

### `list` -- Explore the dataset

List distinct values for any classification field, optionally with neuron counts.

```bash
crantinject list super_class --count
crantinject list cell_type --cell-class kenyon_cell
crantinject list cell_class --super-class sensory --count
```

Valid fields: `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread`

### `inspect` -- View state structure

Display layers, segment counts, and color assignments in a Neuroglancer state.

```bash
crantinject inspect                # reads from clipboard
crantinject inspect -s state.json
```

### `generate` -- Output default template

Print the built-in CRANT scene template to stdout.

```bash
crantinject generate > my_scene.json
```

### `setup` -- Configure credentials

Interactively set your SeaTable API token. Stored in `~/.crantinject/credentials`.

```bash
crantinject setup
```

The token can also be provided via the `CRANTTABLE_TOKEN` environment variable or a file path in `CRANTTABLE_TOKEN_FILE`.


## Testing

```bash
make test
# or
go test ./...
```

## License

License information for this project is not yet specified in a LICENSE file.
