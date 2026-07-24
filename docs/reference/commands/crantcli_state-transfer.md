# crantcli state-transfer

Build a Neuroglancer state from IDs in the clipboard

## Synopsis

Read root IDs from the clipboard, inject them into a Neuroglancer state,
and copy the resulting state URL back to the clipboard.

The clipboard should contain root IDs separated by whitespace, newlines,
or commas. The command loads a base state (from --state, default template,
or user-configured default), injects the IDs into the segmentation layer,
and writes the resulting Neuroglancer URL to the clipboard.

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
  crantcli state-transfer -l "my layer"

  # Attach cell-type labels to the clipboard root IDs
  crantcli state-transfer --labels

  # Write to file instead of clipboard
  crantcli state-transfer -o output.json
```

## Options

```
      --color string          Segment color: named color, 'colored' for random, or hex (#ff0000)
  -h, --help                  help for state-transfer
      --labels                Attach cell-type labels (via an ephemeral secret GitHub gist) so types show next to root IDs in the Seg. panel; requires the gh CLI
      --labels-hook string    Command to publish/clean label sources instead of a GitHub gist (receives info JSON on stdin, prints {"url","id"}); defaults to $CRANT_LABELS_HOOK
      --labels-ttl duration   Delete previously-created label sources older than this on each --labels run (default 168h0m0s)
  -l, --layer string          Target segmentation layer name
  -o, --output string         Output file path (default: clipboard)
  -s, --state string          Base Neuroglancer state (URL or file path; default: template)
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

