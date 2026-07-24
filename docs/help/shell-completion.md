# Shell completion

Cobra can generate completion scripts for Bash, Zsh, Fish, and PowerShell.

=== "Bash"

    Load completion for the current shell:

    ```bash
    source <(crantcli completion bash)
    ```

    To install it persistently, follow the path guidance printed by:

    ```bash
    crantcli completion bash --help
    ```

=== "Zsh"

    ```zsh
    source <(crantcli completion zsh)
    ```

    Put the generated script in a directory on your `$fpath` for persistent completion.

=== "Fish"

    ```fish
    crantcli completion fish | source
    ```

=== "PowerShell"

    ```powershell
    crantcli completion powershell | Out-String | Invoke-Expression
    ```

Completion covers:

- command and flag names;
- valid `list` fields;
- color names and `--color-by` fields;
- CRANT classification values for supported filters.

Classifier completion queries SeaTable, so it needs configured access and a network connection. Static completion remains available without either.

