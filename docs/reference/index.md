# Command reference

The pages in this section are generated directly from the Cobra command tree. Command descriptions, arguments, defaults, and flags therefore share their source with `crantcli --help`.

## Dataset and scene workflows

| Command | Purpose |
| --- | --- |
| [`add`](commands/crantcli_add.md) | Query neurons and inject root IDs into a state, or inject IDs from the clipboard with `--clipboard-ids` |
| [`list`](commands/crantcli_list.md) | List distinct CRANT metadata values |
| [`inspect`](commands/crantcli_inspect.md) | Summarize a Neuroglancer state |
| [`change-def-state`](commands/crantcli_change-def-state.md) | Manage the user-configured default scene, or print the built-in one with `--builtin` |

## CAVE and quality control

| Command | Purpose |
| --- | --- |
| [`check-cave`](commands/crantcli_check-cave.md) | Compare stored and current root IDs |
| [`cave-history`](commands/crantcli_cave-history.md) | Show CAVE merge and split history |
| [`root-info`](commands/crantcli_root-info.md) | Combine CRANT, CAVE, and column context, or find the nearest column from `--pos` |
| [`side-check`](commands/crantcli_side-check.md) | Check side annotations against a nearest reference |

## Configuration and utilities

| Command | Purpose |
| --- | --- |
| [`setup`](commands/crantcli_setup.md) | Store API tokens |
| [`doctor`](commands/crantcli_doctor.md) | Check configuration and, with `--online`, connectivity |
| [`labels`](commands/crantcli_labels.md) | Manage generated label sources, including `--clean` |
| [`completion`](commands/crantcli_completion.md) | Generate shell completion |

!!! info "Guides explain intent"
    Generated reference answers “which flags exist?” The [guides](../guides/query.md) explain how those flags work together in real workflows.
