# Add cell-type labels

Neuroglancer normally displays segment IDs. `--labels` attaches generated segment properties so the Seg. panel can show CRANT cell types beside those IDs.

## Use the default GitHub backend

Install and authenticate the GitHub CLI:

```bash
gh auth login
gh auth status
```

Then add labels while querying:

```bash
crantcli add --cell-type ER --labels
```

Or attach labels to known clipboard IDs:

```bash
crantcli state-transfer --labels
```

Clipboard IDs without CRANT metadata remain unlabeled.

## What gets published

The default backend creates a secret GitHub gist containing:

- each matched root ID;
- a visible `cell_type` label, falling back to `cell_class`;
- filterable tags for cell class, cell instance, side, and super class.

The source URL is attached to the Neuroglancer segmentation layer and recorded locally in `~/.crantcli/label_gists.json`.

!!! warning "Secret gists are unlisted, not private"
    Anyone with the raw source URL can read the published labels and tags. A shared Neuroglancer state contains that URL.

## Automatic cleanup

Each labeled run removes tracked sources older than seven days by default:

```bash
crantcli add --cell-type ER --labels --labels-ttl 72h
```

Clean tracked sources explicitly:

```bash
# Sources older than seven days
crantcli labels clean

# Choose another age
crantcli labels clean --older-than 24h

# Delete every tracked source
crantcli labels clean --all
```

!!! danger "Cleanup changes saved scenes"
    Deleting a published source removes labels from every saved or shared state that still references it. Segment IDs remain, but their generated labels no longer load.

## Use a custom publisher

Provide a hook command with `--labels-hook` or `CRANT_LABELS_HOOK`:

```bash
crantcli add \
  --cell-type ER \
  --labels \
  --labels-hook "/path/to/publisher"
```

For publication, `crantcli` runs:

```text
publisher publish
```

It sends the segment-properties JSON on standard input and expects one JSON object on standard output:

```json
{"url": "https://example.org/source/|neuroglancer-precomputed:", "id": "opaque-handle"}
```

For cleanup it runs:

```text
publisher clean opaque-handle
```

Use the same hook when cleaning hook-published sources.

