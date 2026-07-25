# Configure access

SeaTable provides CRANT metadata. CAVE provides current-root lookups and edit history. `crantcli setup` stores both tokens locally:

```bash
crantcli setup
```

Input is hidden while you type. The CAVE token is optional during setup, so you can add it later by running the command again.

## Which token does each workflow need?

| Workflow | SeaTable | CAVE |
| --- | :---: | :---: |
| Query, list, classify, or locate neurons | ✓ | |
| Add known clipboard IDs to a state | | |
| Add labels to clipboard IDs | ✓ | |
| Check whether root IDs are current | ✓ | ✓ |
| Show CAVE edit history | | ✓ |
| Show combined `root-info` | ✓ | ✓ |

## Non-interactive configuration

CI jobs and scripts can supply raw tokens directly:

```bash
export CRANTTABLE_TOKEN="..."
export CAVE_TOKEN="..."
```

Or point to files containing raw token text:

```bash
export CRANTTABLE_TOKEN_FILE="/run/secrets/cranttable"
export CAVE_TOKEN_FILE="/run/secrets/cave"
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
