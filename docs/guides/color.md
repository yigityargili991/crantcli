# Color a scene

Color can mark one population, distinguish query groups, or expose metadata structure inside a population.

## Apply one color family

```bash
crantcli add --super-class sensory --color blue
```

Named families use a sequence of related tones so repeated additions remain distinguishable:

```text
blue  red  green  turquoise  orange  purple
yellow  pink  brown  indigo  teal  lime
```

You can also provide a six-digit hex value:

```bash
crantcli add --cell-type ER --color "#ff5d73"
```

## Color query groups

Use `colored` to cycle distinct palettes across repeated query groups:

```bash
crantcli add \
  --cell-type ER \
  --cell-type EPG/PEG \
  --color colored
```

With a named family, all groups use tones from that family:

```bash
crantcli add \
  --cell-type ER \
  --cell-type EPG/PEG \
  --color blue
```

## Color by metadata

`--color-by` rebuilds groups from a field in the matched rows:

```bash
crantcli add --super-class sensory --color-by cell_type
crantcli add --region LX --color-by side --color blue
crantcli add --cell-type ER --color-by proofread
```

When you omit `--color`, `--color-by` selects the `colored` palette automatically.

Supported fields:

```text
super_class   cell_class     cell_type       cell_subtype
cell_instance column         side            region
tract         nerve          hemilineage     proofread
```

The derived `column` field comes from `cell_instance`: Δ7 instances use the final four characters; other instances use the final two.

## Sub-color by subtype

Use tones within each query group to distinguish cell subtypes:

```bash
crantcli add \
  --cell-type ER \
  --color blue \
  --color-sub
```

`--color-sub` requires a named color or `colored`. It cannot be combined with `--color-by`, and a single hex color cannot produce subtype variation.

!!! tip "Choose color for the question"
    Use `--color-by` when the metadata field is the analysis dimension. Use repeated query groups when the populations themselves define the comparison.

