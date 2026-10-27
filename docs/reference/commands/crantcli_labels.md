# crantcli labels

Manage label sources created by commands using --labels

## Synopsis

Manage label sources (gists or hook-published) that 'add --labels' and
'cave-history --labels' created and tracked.

With --clean, deletes tracked sources older than --older-than. Use --all to delete
every tracked source regardless of age. Hook-published sources are cleaned via
the same --labels-hook command used to create them. --all, --older-than and
--labels-hook apply only with --clean.

The deprecated 'state-transfer --labels' path still creates these sources the
same way until v0.25.0.

Note: deleting a source removes its labels from any saved/shared state that still
references it.

```
crantcli labels [flags]
```

## Options

```
      --all                   Delete every tracked label source regardless of age
      --clean                 Delete label sources tracked by crantcli
  -h, --help                  help for labels
      --labels-hook string    Hook command used to clean hook-published sources; defaults to $CRANT_LABELS_HOOK
      --older-than duration   Delete tracked sources older than this (ignored with --all) (default 168h0m0s)
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

