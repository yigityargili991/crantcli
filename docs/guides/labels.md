# Label a scene

Neuroglancer normally displays segment IDs. `--labels` attaches generated segment properties so the Seg. panel can show CRANT metadata beside those IDs — cell types by default, or whichever field you name with `--label-by`.

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

## Choose the label field

`--label-by` names the field shown next to each root ID:

```bash
crantcli add --cell-class LNO --labels --label-by cell_subtype
```

Fields after the first are fallbacks, each tried when the previous one is empty. The default, `cell_type,cell_class`, labels by cell type and falls back to cell class:

```bash
# Subtype where annotated, cell type otherwise, cell class as a last resort
crantcli add --cell-class LNO --labels --label-by cell_subtype,cell_type,cell_class
```

## Choose the filterable tags

`--label-tags` names the fields published as filterable chips in the Seg. panel:

```bash
crantcli add --cell-type ER --labels --label-tags cell_subtype,side,region
```

Tag values carry a short field prefix (`subtype_`, `side_`, `region_`), so values from different fields never collide in Neuroglancer's single tag vocabulary. `region` is multi-valued, so a neuron annotated to LX and LW gets both `region_lx` and `region_lw`, and either one filters it. Pass `none` to publish labels without any tags:

```bash
crantcli add --cell-type ER --labels --label-tags none
```

Both flags accept the CRANTb_meta classification fields: `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `cell_instance`, `side`, `region`, `tract`, `nerve`, `hemilineage`, and `proofread`. They work the same way on `state-transfer`:

```bash
crantcli state-transfer --labels --label-by cell_instance,cell_type --label-tags cell_type,side
```

!!! note "Shaping flags need `--labels`"
    `--label-by` and `--label-tags` only shape what `--labels` publishes. Setting them on a run without `--labels` warns and changes nothing.

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

