# crantcli list

List distinct values for a classification field

## Synopsis

List distinct values for a classification field from the CRANT dataset.

Valid fields: super_class, cell_class, cell_type, cell_subtype, cell_instance, side, region, tract, nerve, hemilineage, proofread

```
crantcli list <field> [flags]
```

## Examples

```bash
  crantcli list super_class --count
  crantcli list cell_class --super-class sensory --count
  crantcli list cell_type --cell-class ER
```

## Options

```
      --cell-class string     Filter by cell_class
      --cell-subtype string   Filter by cell_subtype
      --cell-type string      Filter by cell_type
      --count                 Show count of neurons for each value
  -h, --help                  help for list
      --region string         Filter by region
      --side string           Filter by side
      --super-class string    Filter by super_class
      --tract string          Filter by tract
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

