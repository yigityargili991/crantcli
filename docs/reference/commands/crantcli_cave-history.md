# crantcli cave-history

Show CAVE edit history for root IDs

## Synopsis

Show CAVE tabular changelog rows for one or more root IDs.

By default, only edits that affect the final state of the queried root are
included. Use --unfiltered to include broader split/merge history for objects
that were once associated with the queried root.

Use --open or --output to create a fresh Neuroglancer history state. By default,
one compact 3D history layer lists the historical root IDs. Toggle roots in its
Seg. panel to compare stages; only the latest before roots start visible.
Use --compact=false for separate before/after layers with 2D segmentation.
After roots are those reported by CAVE, which may omit split-off objects outside
the queried lineage.

With --open and no --output, the state URL is also copied to the clipboard.
Add --labels to publish editor/time labels using the same hosting as add --labels.

```
crantcli cave-history [root_id...] [flags]
```

## Options

```
      --compact               Use one 3D history layer; set false for separate stages with 2D segmentation (with --open or --output) (default true)
  -h, --help                  help for cave-history
      --history-limit int     Latest edits per root in the viewer state (0 for all; does not limit table/JSON) (default 10)
      --json                  Print JSON output
      --labels                Publish root labels with edit type, editor, and UTC time (requires --open or --output)
      --labels-hook string    Command to publish/clean labels instead of a GitHub gist; defaults to $CRANT_LABELS_HOOK
      --labels-ttl duration   Delete previously-created label sources older than this on each --labels run (default 168h0m0s)
      --open                  Open a colored history state in the default browser
  -o, --output string         Save the history state as Neuroglancer JSON
      --unfiltered            Include unfiltered split/merge history
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

