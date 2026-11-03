# crantcli update

Update crantcli to the newest release

## Synopsis

Check for a newer crantcli release and update in place.

The newest release includes release candidates such as v0.22.4-rc1, so update
installs one as soon as it is published, and moves on to the final release
when that follows. install.sh and install.ps1 keep installing the latest final
release.

The update authenticates the release's platform installer with Sigstore, then
runs it to verify and install the newest binary. It returns only after the
binary has been atomically replaced. Signature verification is built in; no
separate cosign installation is required.

After updating, it prints the CHANGELOG.md sections released since the version
it replaced, or a link to the release page when the changelog cannot be read.

```
crantcli update [flags]
```

## Options

```
  -h, --help   help for update
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

