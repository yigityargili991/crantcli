# crantcli labels

Manage label sources created by commands using --labels

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

