# crantcli completion zsh

Generate the autocompletion script for zsh

## Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(crantcli completion zsh)

To load completions for every new session, execute once:

### Linux:

	crantcli completion zsh > "${fpath[1]}/_crantcli"

### macOS:

	crantcli completion zsh > $(brew --prefix)/share/zsh/site-functions/_crantcli

You will need to start a new shell for this setup to take effect.


```
crantcli completion zsh [flags]
```

## Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

## See also

* [crantcli completion](crantcli_completion.md)	 - Generate the autocompletion script for the specified shell

