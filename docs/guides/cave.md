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
crantcli check-cave --cell-class kenyon_cell
crantcli check-cave --region LX --proofread true
```

Available filters include `super-class`, `cell-class`, `cell-type`, `cell-subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, and `proofread`.

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

Combine CRANT classification, CAVE status, recent history, and nearest-column context:

```bash
crantcli root-info 720575940610453042
crantcli root-info 720575940610453042 --history-limit 10
crantcli root-info 720575940610453042 --json
```

!!! note
    `check-cave` and `root-info` need both SeaTable and CAVE access. `cave-history` needs only CAVE access.

