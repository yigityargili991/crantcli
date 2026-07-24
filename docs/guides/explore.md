# Explore the dataset

Use `list` to discover valid classifier values before building a query.

## List one field

```bash
crantcli list super_class
crantcli list cell_class
crantcli list cell_type
```

Add `--count` to see the number of neurons for each value:

```bash
crantcli list cell_type --count
```

## Narrow the result

Filters let you explore one branch of the classification:

```bash
# Cell classes inside the sensory super-class
crantcli list cell_class --super-class sensory --count

# Cell types inside the Kenyon-cell class
crantcli list cell_type --cell-class kenyon_cell --count

# Regions represented by one cell type
crantcli list region --cell-type EPG/PEG --count
```

Available fields are:

```text
super_class   cell_class     cell_type       cell_subtype
cell_instance side           region          tract
nerve         hemilineage    proofread
```

Region option IDs are resolved to their readable names in the output.

## Let the shell help

Shell completion can suggest commands, flags, fields, colors, and—when credentials and network access are available—classification values.

```bash
source <(crantcli completion zsh)
```

See [Shell completion](../help/shell-completion.md) for persistent setup.

!!! tip
    Start broad with `list ... --count`, then copy a value into an `add` filter. This avoids guessing capitalization or punctuation.

