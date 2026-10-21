# Configure access

SeaTable provides CRANT metadata. CAVE provides current-root lookups and edit history. A second, global CAVE token opens Neuroglancer Share links stored on `global.daf-apis.com`. `crantcli setup` stores these tokens locally:

```bash
crantcli setup
```

Input is hidden while you type. Both CAVE tokens are optional during setup, so you can add them later by running the command again.

Setup then offers to download raider, the small model we fine-tuned for the experimental [`crantcli ask`](../guides/ask.md) command (about 145 MB). Press Enter to download it, or answer `n` and run `crantcli ask --download` later.

## Which token does each workflow need?

| Workflow | SeaTable | CAVE | Global CAVE |
| --- | :---: | :---: | :---: |
| Query, list, classify, or locate neurons | ✓ | | |
| Add known clipboard IDs to a state | | | |
| Add labels to clipboard IDs | ✓ | | |
| Check whether root IDs are current | ✓ | ✓ | |
| Show CAVE edit history | | ✓ | |
| Show combined `root-info` | ✓ | ✓ | |
| Load a [Share link](../concepts/state-resolution.md#share-links) stored on `proofreading.zetta.ai` | | ✓ | |
| Load a [Share link](../concepts/state-resolution.md#share-links) stored on `global.daf-apis.com` | | | ✓ |
| Download the `ask` model and look up table values | ✓ | | |

The CAVE token comes from `proofreading.zetta.ai`, which hosts CRANT. The
global CAVE token comes from `global.daf-apis.com`, the auth server of the state
server that Spelunker's **Share** button uses; copy one from
<https://global.daf-apis.com/sticky_auth/settings/tokens>. crantcli sends each
token only to the host of the server that issued it, not to that host's
subdomains or other ports.

## Non-interactive configuration

CI jobs and scripts can supply raw tokens directly:

```bash
export CRANTTABLE_TOKEN="..."
export CAVE_TOKEN="..."
export CAVE_GLOBAL_TOKEN="..."
```

Or point to files containing raw token text:

```bash
export CRANTTABLE_TOKEN_FILE="/run/secrets/cranttable"
export CAVE_TOKEN_FILE="/run/secrets/cave"
export CAVE_GLOBAL_TOKEN_FILE="/run/secrets/cave-global"
```

CRANT, CAVE, and GitHub token variables are removed from unrelated subprocess
environments, including browser and clipboard helpers. The `gh` publisher
retains its GitHub-specific token variables but never receives CRANT or CAVE
tokens.

### Credential precedence

For each service, `crantcli` uses the first available source:

1. the credential written by `crantcli setup`;
2. the direct token environment variable;
3. the token-file environment variable.

The global CAVE token has a fourth source: the token caveclient saved for
`https://global.daf-apis.com` in
`~/.cloudvolume/secrets/global.daf-apis.com-cave-secret.json`, for example with
`client.auth.save_token`. crantcli reads that file but never writes, moves, or
re-permissions it.

Interactive setup uses the platform credential manager:

| Platform | Storage |
| --- | --- |
| macOS | Keychain |
| Windows | Credential Manager |
| Linux | Secret Service |

If Secret Service is unavailable on Linux, `crantcli` falls back to
base64-encoded files in `~/.crantcli/`. The directory is forced to `0700`, files
are written atomically with `0600` permissions, and symbolic-link paths are
rejected. Base64 is not encryption; the owner-only filesystem permissions
protect this fallback. The fallback remains authoritative until its value has
been written to and read back from Secret Service successfully.

Older credentials in `~/.crantcli/`, `~/.crantinject/`, or
`~/.crant_type_look/` are moved into the platform credential manager and the
old file is removed after a successful migration. On macOS and Windows,
`crantcli` fails closed if the platform credential manager is unavailable; use
the environment-variable or token-file method instead.

## Update or replace a token

Run setup again:

```bash
crantcli setup
```

Because stored credentials take precedence, setting an environment variable does not override an existing setup value. Update the stored credential when you need to rotate it.
