# State resolution

Commands that edit or inspect a Neuroglancer state resolve the input state in the same order.

## Explicit input

When `--state` is present:

1. an existing path is read as a JSON file;
2. otherwise a recognizable Neuroglancer URL is decoded;
3. any other value produces a concrete file-read or JSON error.

```bash
crantcli inspect --state scene.json
crantcli inspect --state "https://spelunker.cave-explorer.org/#!..."
```

## Smart input

Without `--state`, resolution proceeds in this order:

1. non-empty JSON from piped standard input;
2. a Neuroglancer URL on the clipboard;
3. the default state configured with `change-def-state`;
4. the built-in CRANT scene.

```text
stdin ──▶ clipboard URL ──▶ custom default ──▶ built-in scene
```

`--generate` skips the clipboard check and selects the configured or built-in default in normal interactive use. Piped standard input still has priority.

!!! note
    `state-transfer` intentionally treats the clipboard as a list of IDs, so it uses an explicit base state or a default template instead of trying to decode those IDs as a URL.

## Default layer selection

If a command edits segments and `--layer` is absent, it selects the first segmentation layer in the state. Provide the layer name when that choice would be ambiguous:

```bash
crantcli add \
  --cell-type ER \
  --state scene.json \
  --layer "proofreadable seg — SP inputs colored"
```

## Output resolution

`--output` always writes formatted JSON to the specified file.

Without `--output`:

- states loaded from files or standard input are emitted as JSON on standard output;
- states loaded from URLs, the clipboard, or a template are encoded as a Neuroglancer URL and copied to the clipboard;
- if clipboard writing fails, the URL is printed.

`--open` can open the updated URL even when JSON is also written to a file.

