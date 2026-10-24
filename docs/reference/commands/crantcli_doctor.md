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
CAVE answers a root lookup, which requires network access.

The exit status is nonzero when a required check fails, such as a missing
SeaTable token or a custom default state that is not a JSON object. Missing
optional credentials and the ask model only produce warnings or info lines.

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
      --online   Also test SeaTable and CAVE connectivity
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

