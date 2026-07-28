# crantcli update

Update crantcli to the latest release

## Synopsis

Check for a newer crantcli release and update in place.

The update re-runs the platform installer (install.sh on macOS/Linux,
install.ps1 on Windows) for the latest GitHub release, including its checksum
and signature verification. crantcli exits while the installer replaces the
binary, so the new version is available the next time you run crantcli.

```
crantcli update [flags]
```

## Options

```
  -h, --help   help for update
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

