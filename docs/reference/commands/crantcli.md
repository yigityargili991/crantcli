# crantcli

Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

## Synopsis

crantcli queries the CRANT (Clonal Raider Ant Connectome) dataset for neuron root IDs
by classification (super_class, cell_class, cell_type, cell_subtype) and
injects them into a Neuroglancer state JSON.

Run 'crantcli setup' to configure your SeaTable API token.

## Options

```
  -h, --help      help for crantcli
  -v, --version   version for crantcli
```

## See also

* [crantcli add](crantcli_add.md)	 - Query neurons and inject root IDs into a Neuroglancer state
* [crantcli cave-history](crantcli_cave-history.md)	 - Show CAVE edit history for root IDs
* [crantcli change-def-state](crantcli_change-def-state.md)	 - Set the default Neuroglancer state
* [crantcli check-cave](crantcli_check-cave.md)	 - Check if root IDs are still current in CAVE
* [crantcli completion](crantcli_completion.md)	 - Generate the autocompletion script for the specified shell
* [crantcli generate](crantcli_generate.md)	 - Output the default CRANT scene template
* [crantcli inspect](crantcli_inspect.md)	 - Show info about a Neuroglancer state
* [crantcli labels](crantcli_labels.md)	 - Manage cell-type label sources created by commands using --labels
* [crantcli list](crantcli_list.md)	 - List distinct values for a classification field
* [crantcli lookup-column](crantcli_lookup-column.md)	 - Find the closest EPG/PEG neuron's column (region) by position
* [crantcli root-info](crantcli_root-info.md)	 - Show CRANT, CAVE, and nearest-column info for a root ID
* [crantcli setup](crantcli_setup.md)	 - Set or update API tokens (SeaTable and CAVE)
* [crantcli side-check](crantcli_side-check.md)	 - Check selected neuron sides against the nearest EPG/PEG neuron
* [crantcli state-transfer](crantcli_state-transfer.md)	 - Build a Neuroglancer state from IDs in the clipboard

