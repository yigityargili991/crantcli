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

### Credential precedence

For each service, `crantcli` uses the first available source:

1. the credential written by `crantcli setup`;
2. the direct token environment variable;
3. the token-file environment variable.

SeaTable credentials are stored in `~/.crantcli/credentials`; CAVE credentials are stored in `~/.crantcli/cave_credentials`. Stored values are base64-encoded with owner-only file permissions.

!!! warning "Base64 is not encryption"
    Local credential files are obfuscated, not encrypted. Protect access to your user account and never commit token files.

## Update or replace a token

Run setup again:

```bash
crantcli setup
```

Because stored credentials take precedence, setting an environment variable does not override an existing setup value. Update the stored credential when you need to rotate it.

