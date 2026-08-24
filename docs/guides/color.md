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
pos_x         pos_y          pos_z           root_id
```

The derived `column` field comes from `cell_instance`: Δ7 instances use the final four characters; other instances use the final two.

## Color by two fields

Two comma-separated fields nest. The first picks each group's color family, the second varies tone within that family:

```bash
crantcli add --super-class sensory --color-by cell_type,cell_subtype
```

Hue then answers "which cell type", and tone answers "which subtype inside that type". A family carries five tones, so a sixth subtype in one type reuses the first tone.

Two levels need several color families, so this form requires `colored`, which is the default when you omit `--color`. A named family or a hex color offers only one family, so `crantcli` warns and colors by the second field alone.

!!! warning "`--color-sub` is deprecated"
    `--color-sub` encodes two things at once: a palette family per query group, and a tone per `cell_subtype` inside that family. What replaces it depends on where your query groups come from. The flag still works and warns on use.

    With one query group and a named family, `--color-by cell_subtype` produces the same scene:

    ```bash
    # before
    crantcli add --cell-type ER --color blue --color-sub

    # now
    crantcli add --cell-type ER --color blue --color-by cell_subtype
    ```

    With `colored` the two differ. `--color-sub` keeps one family and varies tone, so a query matching seven subtypes renders in five colors and two pairs become indistinguishable. `--color-by cell_subtype` gives each subtype its own hue instead. That separates better and is usually what you want, but it is a different scene, not a reproduction.

    When every query group is one value of the same field, `--color-by <that-field>,cell_subtype` keeps both levels:

    ```bash
    crantcli add --cell-type ER --cell-type EPG/PEG --color-by cell_type,cell_subtype
    ```

    Query groups can also come from a mixed union (`--cell-class LNO --cell-type PEN`) or an `--intersect` cross-product. Those groups are not values of any single field, so no `--color-by` form reproduces them yet; keep `--color-sub` for those.

## Color by position

`pos_x`, `pos_y`, and `pos_z` read the soma position and spread the matched neurons along a ramp instead of sorting them into groups:

```bash
crantcli add --cell-class ER --color-by pos_z
```

The ramp runs from the lowest value in the result to the highest, so each query sets its own endpoints. `colored`, the default, ramps through viridis; a named family ramps through its own tones, dark to light:

```bash
crantcli add --cell-class ER --color-by pos_z --color blue
```

Neurons whose `position` column is empty or malformed have nowhere to sit on the ramp. They take a neutral gray and are counted on their own line.

A continuous field varies per neuron rather than splitting the result into groups, so it cannot join the two-field form.

## Give every neuron its own color

`root_id` puts each neuron in a group of its own, which is what you want when the population itself is the thing you are tracing rather than any annotation on it:

```bash
crantcli add --cell-type ER --color-by root_id
```

`colored` draws from a palette of well-separated hues and, past its length, keeps generating new ones, so large sets stay distinguishable. A named family has only five tones to cycle, so it repeats quickly.

Because every group holds one neuron, `crantcli` reports a total rather than a line per group. `root_id` also works as the second field, giving one hue per outer value and a rotating tone per neuron inside it:

```bash
crantcli add --super-class sensory --color-by cell_type,root_id
```

!!! tip "Choose color for the question"
    Use `--color-by` when the metadata field is the analysis dimension. Use repeated query groups when the populations themselves define the comparison.

