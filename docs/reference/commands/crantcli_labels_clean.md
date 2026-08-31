# crantcli labels clean

Delete label sources (gists or hook-published) tracked by crantcli

## Synopsis

Delete label sources that 'add --labels' or
'state-transfer --labels' created and tracked.

By default, deletes tracked sources older than --older-than. Use --all to delete
every tracked source regardless of age. Hook-published sources are cleaned via
the same --labels-hook command used to create them.

Note: deleting a source removes its labels from any saved/shared state that still
references it.

```
crantcli labels clean [flags]
```

## Options

```
      --all                   Delete every tracked label source regardless of age
  -h, --help                  help for clean
      --labels-hook string    Hook command used to clean hook-published sources; defaults to $CRANT_LABELS_HOOK
      --older-than duration   Delete tracked sources older than this (ignored with --all) (default 168h0m0s)
```

## See also

* [crantcli labels](crantcli_labels.md)	 - Manage label sources created by commands using --labels

