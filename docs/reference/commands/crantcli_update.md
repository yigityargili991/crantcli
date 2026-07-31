# crantcli update

Update crantcli to the latest release

## Synopsis

Check for a newer crantcli release and update in place.

The update authenticates the release's platform installer with Sigstore, then
runs it to verify and install the latest binary. It returns only after the
binary has been atomically replaced. Signature verification is built in; no
separate cosign installation is required.

```
crantcli update [flags]
```

## Options

```
  -h, --help   help for update
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

