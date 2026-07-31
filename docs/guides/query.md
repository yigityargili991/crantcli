# Query and group neurons

`crantcli add` needs at least one filter. It queries matching CRANT rows, removes duplicate root IDs, and adds the remaining IDs to a Neuroglancer segmentation layer.

## Filter a population

```bash
crantcli add --cell-class ER
crantcli add --cell-type ER
crantcli add --super-class sensory --side left
crantcli add --region CX --proofread true
```

The query filters are `--super-class`, `--cell-class`, `--cell-type`, `--cell-subtype`, `--side`, `--region`, `--bundle`, `--tract`, and `--proofread`; `--cell-class`, `--cell-type`, `--cell-subtype`, `--region`, and `--bundle` are repeatable. See [crantcli add](../reference/commands/crantcli_add.md) for the full flag list.

`--region` and `--bundle` express the same filter and cannot be combined in one command.

## Stack populations with union

Cell class, type, and subtype values form separate groups by default. The groups are combined as a union:

```bash
crantcli add \
  --cell-class LNO \
  --cell-subtype PFNc \
  --cell-subtype PFNm3 \
  --cell-type PEN \
  --color-by cell_subtype
```

This loads every matching LNO class, PFNc subtype, PFNm3 subtype, **or** PEN type. If the groups overlap, each root ID appears once.

Other filters apply to every group:

```bash
crantcli add \
  --cell-type ER \
  --cell-type EPG/PEG \
  --side left
```

## Intersect classifier levels

Use `--intersect` when values from two or more classifier levels must all match:

```bash
crantcli add \
  --intersect \
  --cell-class ER \
  --cell-type ER
```

The classifier hierarchy means many cross-level intersections are redundant or empty, so union is normally the useful behavior.

## Choose how segments are inserted

Existing segments are preserved by default:

```bash
crantcli add --cell-type ER
```

Replace the target layer’s current segments:

```bash
crantcli add --cell-type ER --replace
```

Select a specific segmentation layer when a state contains several:

```bash
crantcli add --cell-type ER --layer "merge-biased seg"
```

## Return root IDs only

Skip state manipulation:

```bash
crantcli add --cell-type ER --root-ids-only
```

The IDs are printed one per line and copied to the clipboard. If copying fails,
crantcli warns on standard error and the printed IDs remain usable.

