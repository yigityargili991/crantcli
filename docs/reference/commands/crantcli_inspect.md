# crantcli inspect

Show info about a Neuroglancer state

## Synopsis

Show layers, types, and segment counts for a Neuroglancer state.

Uses smart input: reads from --state flag, stdin, clipboard, or default template.

```
crantcli inspect [flags]
```

## Examples

```bash
  crantcli inspect              # reads from clipboard
  crantcli inspect -s state.json
```

## Options

```
  -h, --help           help for inspect
  -s, --state string   Neuroglancer state (URL or file path)
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

