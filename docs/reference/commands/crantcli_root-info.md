# crantcli root-info

Show CRANT, CAVE, and nearest-column info for a root ID

## Synopsis

Show CRANT metadata, CAVE current-root status, edit history, and nearest EPG/PEG column context for a root ID. With --pos, print only the nearest EPG/PEG column for an x,y,z position.

```
crantcli root-info [root_id] [flags]
```

## Options

```
  -h, --help                help for root-info
      --history-limit int   Number of recent CAVE history entries to print (default 5)
      --json                Print JSON output
      --pos string          Position as x,y,z (e.g. 100.5,200.3,300.1)
      --unfiltered          Include unfiltered split/merge history
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

