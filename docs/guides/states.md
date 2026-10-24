# Work with Neuroglancer states

State-editing commands accept Neuroglancer URLs, JSON files, piped JSON, and clipboard URLs.

## Use an explicit URL or file

```bash
crantcli add --cell-type ER --state state.json --output result.json
crantcli add --cell-type ER --state "https://spelunker.cave-explorer.org/#!..."
```

An existing file takes precedence even if its filename contains URL-like text.

A link from the viewer's **Share** button works too, in the clipboard or with
`--state`: crantcli fetches the state it points to.
[Share links](../concepts/state-resolution.md#share-links) explains which
state servers it can sign in to.

## Pipe a state

```bash
crantcli change-def-state --builtin \
  | crantcli add --cell-type ER \
  > er-state.json
```

Piped JSON is read before the clipboard.

## Inspect a state

```bash
crantcli inspect --state result.json
```

`inspect` reports each layer’s name, type, and source, plus segment and color-entry counts for segmentation layers.

## Transfer known IDs from the clipboard

Copy root IDs separated by spaces, commas, or newlines, then run:

```bash
crantcli add --clipboard-ids
```

The IDs are deduplicated and appended to the target segmentation layer. Pass
`--replace` to replace the layer’s existing segment list instead (the retired
`state-transfer` command always replaced). Choose the base state and output
explicitly when needed:

```bash
crantcli add --clipboard-ids \
  --state base.json \
  --output result.json \
  --layer "proofreadable seg" \
  --color turquoise
```

!!! warning
    `add --clipboard-ids --replace` replaces the selected layer’s existing
    segment list. Keep a copy of the base state if you may need that selection
    later.

## Configure a reusable default

```bash
crantcli change-def-state /path/to/preferred-state.json
crantcli change-def-state --show
crantcli change-def-state --reset
```

The configured JSON is stored at `~/.crantcli/default_state.json`.

## Generate the built-in state

Write the embedded CRANT scene to a file:

```bash
crantcli change-def-state --builtin > crant-scene.json
```

## Understand output behavior

With `--output`, the result is always JSON written to that file. Without it:

| Input source | Default output |
| --- | --- |
| Clipboard URL, explicit URL, or template | Neuroglancer URL copied to the clipboard |
| File or piped JSON | Formatted JSON on standard output |

If clipboard writing fails, crantcli warns and prints the URL to standard
output. The command still succeeds, because the URL reached you; it fails only
if no destination could be written at all.

`add --clipboard-ids` follows these output rules. The deprecated
`state-transfer` command is the exception: without `--output` it always copies
a URL to the clipboard, whatever the input source. Linux Wayland/X11 clipboard
support is built into the binary; external clipboard tools are compatibility
fallbacks only.

[Learn the full resolution order](../concepts/state-resolution.md)
