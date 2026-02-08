# crant_type_look

A CLI tool for querying the [CRANT](https://github.com/flyconnectome/crant) (Connectome Reconstruction and Analysis of Neural Tissue) dataset and injecting neuron root IDs into [Neuroglancer](https://github.com/google/neuroglancer) visualization states.

Query neurons by classification, get their root IDs into a Neuroglancer scene, and open it in your browser -- all in one command.

## Quick Start

```bash
# Configure your SeaTable API token
crant_type_look setup

# Query neurons and inject into a Neuroglancer state (reads/writes clipboard)
crant_type_look add --cell-class kenyon_cell

# Open the result directly in your browser
crant_type_look add --cell-type ER --open
```

## Installation

### From source

Requires Go 1.25.5+.

```bash
git clone https://github.com/yigityargili991/crant_type_look.git
cd crant_type_look
make build      # produces ./crant_type_look
make install    # installs to $GOBIN (see `go env GOBIN`, defaults to $GOPATH/bin)
```

### From releases

Pre-built binaries for Linux, macOS, and Windows (amd64/arm64) are available on the [Releases](https://github.com/yigityargili991/crant_type_look/releases) page.

## Commands

### `add` -- Query and inject neurons

The main command. Queries the CRANT dataset and injects matching root IDs into a Neuroglancer state.

```bash
# Smart mode: reads state from clipboard, injects, copies back
crant_type_look add --cell-class kenyon_cell

# Explicit file I/O
crant_type_look add --cell-class kenyon_cell -s state.json -o modified.json

# Start from the default CRANT scene template
crant_type_look add --cell-class kenyon_cell --generate

# Combine filters
crant_type_look add --super-class sensory --side left --color "#ff0000"

# Replace segments instead of appending
crant_type_look add --cell-type ER --replace

# Just get root IDs, no state manipulation
crant_type_look add --cell-class kenyon_cell --root-ids-only
```

**Filter flags:** `--super-class`, `--cell-class`, `--cell-type`, `--cell-subtype`, `--side`, `--region`, `--tract`, `--proofread`

**State flags:** `-s`/`--state` (URL or file), `-g`/`--generate` (use default template), `-o`/`--output` (file path), `-l`/`--layer` (target layer name), `--color` (hex color), `--replace`, `--open`

**Smart input resolution** (when no `--state` is given):
1. stdin (piped JSON)
2. Clipboard (Neuroglancer URL)
3. Last state URL from a previous session
4. Default CRANT scene template

### `list` -- Explore the dataset

List distinct values for any classification field, optionally with neuron counts.

```bash
crant_type_look list super_class --count
crant_type_look list cell_type --cell-class kenyon_cell
crant_type_look list cell_class --super-class sensory --count
```

Valid fields: `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread`

### `inspect` -- View state structure

Display layers, segment counts, and color assignments in a Neuroglancer state.

```bash
crant_type_look inspect                # reads from clipboard
crant_type_look inspect -s state.json
```

### `generate` -- Output default template

Print the built-in CRANT scene template to stdout.

```bash
crant_type_look generate > my_scene.json
```

### `setup` -- Configure credentials

Interactively set your SeaTable API token. Stored in `~/.crant_type_look/credentials`.

```bash
crant_type_look setup
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
