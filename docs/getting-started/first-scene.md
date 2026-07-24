# Build your first scene

Start from the built-in CRANT scene, find all ER neurons, color them by cell type, and open the result:

```bash
crantcli add \
  --cell-type ER \
  --color-by cell_type \
  --generate \
  --open
```

The command reports matches on standard error and produces an updated Neuroglancer state.

## Reuse the scene from your clipboard

Once a Neuroglancer URL is on your clipboard, the next query can add another population to it:

```bash
crantcli add --cell-type ER --color purple
```

With no `--state`, `crantcli` checks the clipboard before falling back to your configured or built-in default scene. The updated URL is copied back to the clipboard.

## Use files instead

For a reproducible workflow, provide both paths:

```bash
crantcli add \
  --cell-type ER \
  --state base-state.json \
  --output er-state.json
```

Inspect the result without opening a browser:

```bash
crantcli inspect --state er-state.json
```

## Get IDs without editing a scene

```bash
crantcli add --cell-type ER --root-ids-only
```

Root IDs are printed and copied to the clipboard when clipboard access is available.

## Where to go next

- [Explore the dataset](../guides/explore.md) before choosing filters.
- [Query and group neurons](../guides/query.md) for union and intersection behavior.
- [Color a scene](../guides/color.md) by metadata.
- [Understand state resolution](../concepts/state-resolution.md) when mixing files, pipes, and clipboard input.
