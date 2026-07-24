# Command reference

The pages in this section are generated directly from the Cobra command tree. Command descriptions, arguments, defaults, and flags therefore share their source with `crantcli --help`.

## Dataset and scene workflows

| Command | Purpose |
| --- | --- |
| [`add`](commands/crantcli_add.md) | Query neurons and inject root IDs into a state |
| [`list`](commands/crantcli_list.md) | List distinct CRANT metadata values |
| [`inspect`](commands/crantcli_inspect.md) | Summarize a Neuroglancer state |
| [`state-transfer`](commands/crantcli_state-transfer.md) | Build a state from clipboard root IDs |
| [`generate`](commands/crantcli_generate.md) | Print the built-in scene |
| [`change-def-state`](commands/crantcli_change-def-state.md) | Manage the user-configured default scene |

## CAVE and quality control

| Command | Purpose |
| --- | --- |
| [`check-cave`](commands/crantcli_check-cave.md) | Compare stored and current root IDs |
| [`cave-history`](commands/crantcli_cave-history.md) | Show CAVE merge and split history |
| [`root-info`](commands/crantcli_root-info.md) | Combine CRANT, CAVE, and column context |
| [`lookup-column`](commands/crantcli_lookup-column.md) | Find the nearest EPG/PEG column |
| [`side-check`](commands/crantcli_side-check.md) | Check side annotations against a nearest reference |

## Configuration and utilities

| Command | Purpose |
| --- | --- |
| [`setup`](commands/crantcli_setup.md) | Store API tokens |
| [`labels`](commands/crantcli_labels.md) | Manage generated label sources |
| [`labels clean`](commands/crantcli_labels_clean.md) | Delete tracked label sources |
| [`completion`](commands/crantcli_completion.md) | Generate shell completion |

!!! info "Guides explain intent"
    Generated reference answers “which flags exist?” The [guides](../guides/query.md) explain how those flags work together in real workflows.

