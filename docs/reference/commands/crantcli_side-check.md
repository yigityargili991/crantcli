# crantcli side-check

Check selected neuron sides against the nearest EPG/PEG neuron

## Synopsis

Check selected neurons against the nearest valid EPG/PEG neuron by 3D
Euclidean distance. Prints one selected root_id per line when the selected
neuron has missing side data, missing or malformed position data, or the same
side as the nearest valid EPG/PEG neuron.

```
crantcli side-check [flags]
```

## Options

```
      --cell-class string   Check neurons with this cell_class
      --cell-type string    Check neurons with this cell_type
  -h, --help                help for side-check
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

