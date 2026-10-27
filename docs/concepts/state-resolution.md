# State resolution

Commands that edit or inspect a Neuroglancer state resolve the input state in the same order.

## Explicit input

When `--state` is present:

1. an existing path is read as a JSON file;
2. otherwise a recognizable Neuroglancer URL is decoded, or for a
   [Share link](#share-links), the state it points to is fetched;
3. any other value produces a concrete file-read or JSON error.

```bash
crantcli inspect --state scene.json
crantcli inspect --state "https://spelunker.cave-explorer.org/#!..."
```

## Smart input

Without `--state`, resolution proceeds in this order:

1. non-empty JSON from piped standard input;
2. a Neuroglancer URL or [Share link](#share-links) on the clipboard;
3. the default state configured with `change-def-state`;
4. the built-in CRANT scene.

```text
stdin ──▶ clipboard URL ──▶ custom default ──▶ built-in scene
```

`--generate` skips the clipboard check and selects the configured or built-in
default in normal interactive use. Piped standard input still has priority. If
clipboard access is unavailable, crantcli warns before using the default. A
clipboard value that looks like a Neuroglancer URL but has no state fragment —
the bare viewer link — also falls back to the default with a warning, while one
whose fragment contains malformed state is reported as an error rather than
silently ignored.

!!! note
    `add --clipboard-ids` intentionally treats the clipboard as a list of IDs, so it uses `--state`, piped JSON, or a default template as the base state instead of trying to decode those IDs as a URL.

!!! note
    `check-cave --refresh-state` updates a state you supply, so it stops at the clipboard step: without `--state`, piped JSON, or a clipboard URL it reports an error instead of using a default.

## Share links

The viewer's **Share** button copies a link that names where a state is stored
instead of carrying it, such as
`https://spelunker.cave-explorer.org/#!middleauth+https://global.daf-apis.com/nglstate/api/v1/<id>`.
crantcli fetches that state and uses it as it would a state decoded from a URL,
reporting for example
`State loaded from clipboard (resolved Share link "middleauth+https://…")`.
Output written as a URL embeds the state; nothing is uploaded to a state
server.

State servers require signing in, and crantcli attaches a token only for the
host of the auth server that issued it, on the default https port: the CAVE
token for `proofreading.zetta.ai`, and the
[global CAVE token](../getting-started/configure.md#which-token-does-each-workflow-need)
for `global.daf-apis.com`, where Spelunker's **Share** button stores states.
Subdomains and other ports get no token.
States stored anywhere other than `https://`, such as `gs://`, are not fetched,
nor are states on a host whose name contains non-ASCII characters, which can
imitate another host's name.
A link that cannot be loaded is an error that names it, not a reason to fall
back to a default state; see
[A Neuroglancer Share link does not load](../help/troubleshooting.md#a-neuroglancer-share-link-does-not-load).

## Default layer selection

If a command edits segments and `--layer` is absent, it selects the first segmentation layer in the state. Provide the layer name when that choice would be ambiguous. An exact name wins; otherwise a name that differs only in surrounding spaces matches, so `--layer "proofreadable seg"` finds the built-in scene's `proofreadable seg ` layer:

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
- if clipboard writing fails, a warning is emitted and the URL is printed to standard output; the command succeeds, since the result still reached you. It fails only when no destination at all could be written.

Clipboard copy, file/stdout output, and `--open` are independent delivery
actions: failure in one does not suppress the others.

`--open` can open the updated URL even when JSON is also written to a file.

