---
hide:
  - toc
---

<div class="manual-cover" markdown>
<div class="manual-cover__bar" aria-hidden="true">
  <span>CRANTCLI(1)</span>
  <span>USER COMMANDS</span>
  <span>CRANTCLI(1)</span>
</div>

# crantcli

Query CRANT neurons, resolve current root IDs, and write Neuroglancer states from the command line.

`crantcli <command> [flags]`

[Get started](getting-started/index.md) · [Command reference](reference/index.md)
</div>

## Operations

The interface is command-first. Start with the operation that matches the state you have.

| Command | Operation |
| --- | --- |
| [`list`](guides/explore.md) | Inspect classifications, regions, and counts |
| [`add`](guides/query.md) | Query populations and inject their root IDs |
| [`check-cave`](guides/cave.md) | Detect stale IDs and refresh a scene |
| [`inspect`](guides/states.md) | Work with state JSON through files, streams, or the clipboard |

## Minimum path

With `crantcli` installed, store your access tokens and open a first scene:

```bash
crantcli setup
crantcli add \
  --cell-class kenyon_cell \
  --generate \
  --color-by cell_type \
  --open
```

Clipboard automation is optional. Every state-editing workflow also accepts explicit input and output files.

[Installation and prerequisites](getting-started/install.md)
