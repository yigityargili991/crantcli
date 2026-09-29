# Check CAVE freshness

Proofreading merges and splits can change a neuron’s root ID. `check-cave` uses the stored supervoxel ID to ask CAVE for the current root and compare it with CRANT.

## Check specific roots

```bash
crantcli check-cave 720575940610453042
crantcli check-cave 720575940610453042 720575940631928371
```

## Check a population

Check every CRANT row:

```bash
crantcli check-cave --all
```

Or use filter flags without `--all`:

```bash
crantcli check-cave --cell-type ER
crantcli check-cave --region LX --proofread true
```

Filters: `super-class`, `cell-class`, `cell-type`, `cell-subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread`.

Root ID arguments cannot be combined with `--all` or filter flags.

## Produce a stale mapping

```bash
crantcli check-cave --all --mapping
```

Each stale result is printed as:

```text
old_root_id<TAB>current_cave_root_id
```

For automation, quiet mode prints only stale entries and exits with status `1` when it finds any:

```bash
crantcli check-cave --all --quiet
```

## Refresh a Neuroglancer state

Replace stale segment IDs while leaving current and unrelated IDs intact:

```bash
crantcli check-cave \
  --all \
  --refresh-state \
  --state state.json \
  --output refreshed.json
```

Target one segmentation layer:

```bash
crantcli check-cave \
  --all \
  --refresh-state \
  --state state.json \
  --output refreshed.json \
  --layer "proofreadable seg"
```

When combining `--mapping` with `--refresh-state`, provide `--output` so mapping text and state JSON do not share standard output.

## Inspect history and metadata

Show edit history:

```bash
crantcli cave-history 720575940610453042
crantcli cave-history 720575940610453042 --json
```

By default the history is limited to edits affecting the queried root’s final state. Add `--unfiltered` for broader split and merge history.

### Open a colored history state

```bash
crantcli cave-history 576460752688642351 --open
crantcli cave-history 576460752688642351 --open --labels
crantcli cave-history 576460752688642351 --output history.json --history-limit 20
crantcli cave-history 576460752688642351 --open --compact=false
```

`--open` or `--output` builds a fresh CRANT scene with one **Edit history** layer
and opens its Seg. panel. Each historical root appears once in the list, with a
distinct color and its own visibility toggle. The latest before roots start
visible; the other roots remain listed but hidden. If the latest edit has no
before roots, its reported after roots start visible instead. A queried root
without edits also starts visible.

Add `--labels` to put the operation, editor, and UTC time beside each root in
this list. Hide the currently visible roots before showing another stage to
avoid overlapping geometry.

The default compact view shows **3D meshes only** for history. Its segmentation
graph and 2D overlay are disabled so overlapping historical roots remain
independent. Use `--compact=false` for separate queried-root, **before**, and
**after (reported)** layers with 2D segmentation. In that view, layer names carry
the edit details and only the latest before layer starts visible.

The viewer includes the latest 10 edits per queried root by default. Use
`--history-limit 0` for all edits; large histories produce longer root lists and
larger state URLs. This limit does not change table/JSON output or reduce the history
requested from CAVE. `--json` cannot be combined with `--open` or `--output`.
Without `--output`, `--open` also copies the state URL to the clipboard.

`--labels` adds editor/time labels next to segment IDs in the Seg. panel, using
the same [label hosting and cleanup](labels.md) as `add --labels`. Labels include
the before/after role, operation type and ID, editor (or user ID when no name is
available), and UTC time. When a root participates in several displayed edits,
its label lists each one. Use `--labels-hook` or `CRANT_LABELS_HOOK` for a custom
publisher, and `--labels-ttl` to set the cleanup age.

The state uses historical **root IDs**, whose geometry represents the object at
that stage. Raw supervoxels in a normal Graphene layer resolve to current roots.
The after stages show only IDs returned by CAVE's changelog; split-off objects
outside the queried lineage may be absent. Historical mesh availability depends
on the dataset server. Toggle one stage at a time when comparing edits.

### Combine history with metadata

Combine CRANT classification, CAVE status, recent history, and nearest-column context:

```bash
crantcli root-info 720575940610453042
crantcli root-info 720575940610453042 --history-limit 10
crantcli root-info 720575940610453042 --json
```

!!! note
    `check-cave` and `root-info` need both SeaTable and CAVE access. `cave-history` needs only CAVE access.
