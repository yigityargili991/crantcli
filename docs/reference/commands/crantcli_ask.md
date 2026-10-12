# crantcli ask

Turn a plain-English request into a crantcli command (experimental)

## Synopsis

Turn a plain-English request into a crantcli command with raider, a small
model we fine-tuned for crantcli (from SmolLM2-135M-Instruct) that runs on
your computer.

ask is experimental: the model can suggest the wrong command, flags, or
values, so check the suggestion before running it.

ask prints the suggested command and asks before running it. Table values
mentioned in the request are looked up in the CRANT table (cached for a day),
and the model can only write flags and values the request gives evidence for.

The first run offers to download the raider model (about 145 MB) into the user
cache directory. The download is encrypted and only unlocks with a SeaTable
token for the CRANT base. Use --download to install or update it without a
request, or --model / CRANTCLI_ASK_MODEL to use another model file.

```
crantcli ask <request> [flags]
```

## Examples

```bash
  crantcli ask "show the PEN neurons colored by proofreading status and open them"
  crantcli ask --print "which column is 576460752669867599 in"
  crantcli ask -y "edit history of 576460752669867599 as json"
  crantcli ask --download
```

## Options

```
      --download       Install the raider model without asking
  -h, --help           help for ask
      --model string   Path to another model file (GGUF)
      --print          Only print the suggested command
      --refresh        Fetch table values again instead of using the cache
  -y, --yes            Run the suggested command without asking
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

