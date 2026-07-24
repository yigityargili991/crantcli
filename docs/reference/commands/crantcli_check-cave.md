# crantcli check-cave

Check if root IDs are still current in CAVE

## Synopsis

Check whether root IDs stored in SeaTable still match the current CAVE
chunkedgraph by looking up each neuron's supervoxel_id.

Supervoxel IDs are stable, but root IDs can change when proofreading edits
(merges/splits) happen in CAVE. This command detects stale root IDs.

```
crantcli check-cave [root_id...] [flags]
```

## Examples

```bash
  # Check a single root ID
  crantcli check-cave 720575940610453042

  # Check multiple root IDs
  crantcli check-cave 720575940610453042 720575940631928371

  # Check all neurons in the table
  crantcli check-cave --all

  # Check only kenyon cells
  crantcli check-cave --all --cell-class kenyon_cell

  # Only print stale entries (exit code 1 if any found)
  crantcli check-cave --all --quiet

  # Print stale root mappings as old_root_id<TAB>current_cave_root_id
  crantcli check-cave --all --mapping

  # Refresh stale segment IDs in a Neuroglancer state
  crantcli check-cave --all --refresh-state -s state.json -o refreshed.json
```

## Options

```
      --all                   Check all neurons (or filtered subset)
      --cell-class string     Filter by cell_class
      --cell-subtype string   Filter by cell_subtype
      --cell-type string      Filter by cell_type
  -h, --help                  help for check-cave
      --hemilineage string    Filter by hemilineage
  -l, --layer string          Target segmentation layer name for --refresh-state
      --mapping               Print stale mappings as old_root_id<TAB>current_cave_root_id
      --nerve string          Filter by nerve
  -o, --output string         Output file path for --refresh-state (default: clipboard or stdout)
      --proofread string      Filter by proofread status
  -q, --quiet                 Suppress progress/summary; only output stale entries; exit code 1 if any found
      --refresh-state         Replace stale segment IDs in a Neuroglancer state with current CAVE root IDs
      --region string         Filter by region
      --side string           Filter by side
  -s, --state string          Neuroglancer state (URL or file path) for --refresh-state
      --super-class string    Filter by super_class
      --tract string          Filter by tract
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

