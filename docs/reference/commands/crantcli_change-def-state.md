# crantcli change-def-state

Set the default Neuroglancer state

## Synopsis

Set or update the default Neuroglancer JSON state used when no other state
source is available (no --state, piped JSON, or clipboard URL).

Pass the full JSON state as an argument, or pass a path to a JSON file.

```
crantcli change-def-state <json-state> [flags]
```

## Examples

```bash
  crantcli change-def-state "$(crantcli generate)"
  crantcli change-def-state /path/to/state.json
  crantcli change-def-state --show
  crantcli change-def-state --reset
```

## Options

```
  -h, --help    help for change-def-state
      --reset   Reset to built-in default
      --show    Show the current default state
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

