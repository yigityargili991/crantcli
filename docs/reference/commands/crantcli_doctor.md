# crantcli doctor

Report configuration health and test connectivity

## Synopsis

Report whether crantcli can find the configuration it needs: the SeaTable
token, the CAVE token, the global CAVE token, the default Neuroglancer state,
and the optional crantcli ask model.

doctor is read-only. It never prints a token value and never changes your
configuration. Each check prints one line with a status of ok, warn, info, or
error.

By default doctor only inspects local configuration. With --online it also
checks that the configured SeaTable token exchanges for a session and that
CAVE answers a root lookup, which requires network access. An online check
whose token is not configured reports an info line instead of an error.

The exit status is nonzero when a required check fails, such as a missing
SeaTable token. Missing optional credentials and the ask model only produce
warnings or info lines. A custom default state that cannot be read or is not a
JSON object only produces a warning: crantcli ignores it and uses the built-in
template.

```
crantcli doctor [flags]
```

## Examples

```bash
  # Check the local configuration
  crantcli doctor

  # Also test that the SeaTable token exchanges and CAVE answers queries
  crantcli doctor --online
```

## Options

```
  -h, --help     help for doctor
      --online   Also test SeaTable and CAVE connectivity; checks without a token are skipped
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

