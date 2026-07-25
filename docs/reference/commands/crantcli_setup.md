# crantcli setup

Set or update API tokens (SeaTable and CAVE)

## Synopsis

Interactively set or update the SeaTable API token and optional CAVE token.

Tokens are stored in ~/.crantcli/ as base64-encoded files with 0600 permissions
(directory 0700). Note that base64 is obfuscation, not encryption: the file
permissions are what protect the token, and crantcli tightens them back to
0600 if it finds them looser.

```
crantcli setup [flags]
```

## Options

```
  -h, --help   help for setup
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

