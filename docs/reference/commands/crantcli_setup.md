# crantcli setup

Set or update API tokens (SeaTable and CAVE)

## Synopsis

Interactively set or update the SeaTable API token, the optional CAVE token,
and the optional global CAVE token, then offer to download the model for the
experimental crantcli ask command.

The CAVE token, issued by proofreading.zetta.ai, serves the CAVE commands and
Neuroglancer Share links stored on proofreading.zetta.ai. The global CAVE token,
issued by global.daf-apis.com, is sent only there, for Share links stored on
global.daf-apis.com. Without one, crantcli reads the token caveclient saved in
~/.cloudvolume/secrets/global.daf-apis.com-cave-secret.json, a file it never
writes.

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

