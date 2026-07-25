# crantcli setup

Set or update API tokens (SeaTable and CAVE)

## Synopsis

Interactively set or update the SeaTable API token and optional CAVE token.

Tokens are stored in the operating system's secure credential manager: Keychain
on macOS, Credential Manager on Windows, and Secret Service on Linux. If Secret
Service is unavailable on Linux, crantcli uses an owner-only file in ~/.crantcli/.
Existing file-based credentials are migrated automatically when possible.

```
crantcli setup [flags]
```

## Options

```
  -h, --help   help for setup
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

