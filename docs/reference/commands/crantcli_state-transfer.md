# crantcli state-transfer

Build a Neuroglancer state from IDs in the clipboard

## Synopsis

Read root IDs from the clipboard, inject them into a Neuroglancer state,
and copy the resulting state URL back to the clipboard (overwriting it).

The clipboard should contain root IDs separated by whitespace, newlines,
or commas. The command loads a base state (from --state, piped stdin,
user-configured default, or built-in template), injects the IDs into the
segmentation layer, and writes the resulting Neuroglancer URL to the clipboard.

Note: with --labels, the clipboard root IDs are sent to SeaTable to look up
their metadata, and the matching rows are published as an unlisted gist
(or via --labels-hook/$CRANT_LABELS_HOOK). --label-by chooses the field shown
as the label and --label-tags the fields published as filterable chips.

```
crantcli state-transfer [flags]
```

## Examples

```bash
  # Copy some root IDs, then:
  crantcli state-transfer

  # Use a specific base state
  crantcli state-transfer -s base.json

  # Target a specific segmentation layer
  crantcli state-transfer -l "merge-biased seg"

  # Attach cell-type labels to the clipboard root IDs
  crantcli state-transfer --labels

  # Label by cell_subtype instead, and filter by cell_type
  crantcli state-transfer --labels --label-by cell_subtype,cell_type --label-tags cell_type,side

  # Write to file instead of clipboard
  crantcli state-transfer -o output.json
```

## Options

```
      --color string          Segment color: named color, 'colored' for random, or hex (#ff0000)
  -h, --help                  help for state-transfer
      --label-by string       Field shown as the --labels label: super_class, cell_class, cell_type, cell_subtype, cell_instance, side, region, tract, nerve, hemilineage, proofread. Further comma-separated fields are fallbacks, each tried when the previous one is empty (default "cell_type,cell_class")
      --label-tags string     Comma-separated fields published as filterable tag chips (super_class, cell_class, cell_type, cell_subtype, cell_instance, side, region, tract, nerve, hemilineage, proofread), or 'none' for no tags (default "cell_class,cell_instance,side,super_class")
      --labels                Attach cell-type labels (via an ephemeral secret GitHub gist) so types show next to root IDs in the Seg. panel; requires the gh CLI, or a publish hook via --labels-hook/$CRANT_LABELS_HOOK
      --labels-hook string    Command to publish/clean label sources instead of a GitHub gist (receives info JSON on stdin, prints {"url","id"}); defaults to $CRANT_LABELS_HOOK
      --labels-ttl duration   Delete previously-created label sources older than this on each --labels run (default 168h0m0s)
  -l, --layer string          Target segmentation layer name
  -o, --output string         Output file path (default: clipboard)
  -s, --state string          Base Neuroglancer state (URL or file path; default: template)
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

