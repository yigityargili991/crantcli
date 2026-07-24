# CRANT classifications

CRANT metadata describes each neuron through related classification and annotation fields.

## Classification hierarchy

The main classifier levels move from broad to specific:

```text
super_class
  └─ cell_class
       └─ cell_type
            └─ cell_subtype
```

`cell_instance` identifies a more specific instance; its suffix encodes the column (`--color-by column`).

Because the levels are hierarchical, combining a class with one of its own types as an intersection is often redundant. This is why repeated `cell_class`, `cell_type`, and `cell_subtype` values use union behavior by default in `add`.

## Annotation fields

Other fields describe where a neuron belongs or the state of its annotation:

| Field | Meaning in a query |
| --- | --- |
| `side` | Side annotation |
| `region` | Region or bundle annotation |
| `tract` | Tract annotation |
| `nerve` | Nerve annotation |
| `hemilineage` | Hemilineage annotation |
| `proofread` | Proofreading status |

Not every command exposes every field as a filter. For example, `add` filters by region and tract but can color results by nerve or hemilineage.

## Query groups

These flags are repeatable in `add`:

```text
--cell-class
--cell-type
--cell-subtype
```

Each value creates a separate query group, and the groups are united:

```bash
crantcli add \
  --cell-type ER \
  --cell-type EPG/PEG \
  --color colored
```

Scalar filters such as `--side left` apply to every group.

`--intersect` changes the behavior only when two or more classifier levels are present:

```bash
crantcli add \
  --intersect \
  --cell-class ER \
  --cell-type ER
```

## Discover current values

Classifier values come from the live CRANT table and may evolve. Query them instead of relying on a frozen list:

```bash
crantcli list cell_class --count
crantcli list cell_type --cell-class ER --count
```

