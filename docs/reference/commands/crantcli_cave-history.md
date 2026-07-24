# crantcli cave-history

Show CAVE edit history for root IDs

## Synopsis

Show CAVE tabular changelog rows for one or more root IDs.

By default, only edits that affect the final state of the queried root are
included. Use --unfiltered to include broader split/merge history for objects
that were once associated with the queried root.

```
crantcli cave-history [root_id...] [flags]
```

## Options

```
  -h, --help         help for cave-history
      --json         Print JSON output
      --unfiltered   Include unfiltered split/merge history
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

