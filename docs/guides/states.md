# Work with Neuroglancer states

State-editing commands accept Neuroglancer URLs, JSON files, piped JSON, and clipboard URLs.

## Use an explicit URL or file

```bash
crantcli add --cell-type ER --state state.json --output result.json
crantcli add --cell-type ER --state "https://spelunker.cave-explorer.org/#!..."
```

An existing file takes precedence even if its filename contains URL-like text.

## Pipe a state

```bash
crantcli generate \
  | crantcli add --cell-type ER \
  > er-state.json
```

Piped JSON is read before the clipboard.

## Inspect a state

```bash
crantcli inspect --state result.json
```

`inspect` reports each layer’s name, type, source, segment count, and color-entry count.

## Transfer known IDs from the clipboard

Copy root IDs separated by spaces, commas, or newlines, then run:

```bash
crantcli state-transfer
```

The IDs are deduplicated and replace the target segmentation layer’s segment list. Choose the base state and output explicitly when needed:

```bash
crantcli state-transfer \
  --state base.json \
  --output result.json \
  --layer "proofreadable seg" \
  --color turquoise
```

!!! warning
    `state-transfer` replaces the selected layer’s existing segment list. Keep a copy of the base state if you may need that selection later.

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
crantcli generate > crant-scene.json
```

## Understand output behavior

With `--output`, the result is always JSON written to that file. Without it:

| Input source | Default output |
| --- | --- |
| Clipboard URL, explicit URL, or template | Neuroglancer URL copied to the clipboard |
| File or piped JSON | Formatted JSON on standard output |

If clipboard writing fails, URL output falls back to standard output.

[Learn the full resolution order](../concepts/state-resolution.md)
