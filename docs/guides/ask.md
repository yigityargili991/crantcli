# Ask in plain English

!!! warning "Experimental"
    `crantcli ask` is experimental. The model can suggest the wrong command,
    flags, or values, so read the suggestion before you run it.

`ask` turns a plain-English request into a crantcli command with raider, a
small model we fine-tuned for crantcli that runs on your computer. raider is
based on [SmolLM2-135M-Instruct](https://huggingface.co/HuggingFaceTB/SmolLM2-135M-Instruct)
(Apache License 2.0) and trained on thousands of example requests for crantcli
commands:

```console
$ crantcli ask "show the PEN neurons colored by proofreading status and replace the scene"
Warning: crantcli ask is experimental and can suggest the wrong command. Check it before running it.
crantcli add --cell-type PEN --color-by proofread --replace
Run it? [y/N]
```

Answer `y` to run the suggestion. Anything else stops without running it.

## Install the model

`crantcli setup` offers to download the raider model (about 145 MB) as its
last step. If you skipped it, the first `ask` offers it again:

```console
$ crantcli ask "which column is 576460752669867599 in"
crantcli ask (experimental) turns a plain-English request into a crantcli command, using raider, a small model we fine-tuned for crantcli that runs on your computer.
Download the raider model v4 (145 MB) now? [Y/n]
```

The download is encrypted and only unlocks with a SeaTable token for the CRANT
base, so it needs the same setup as the other commands (`crantcli setup`). The
model is stored in your user cache directory and checked against a checksum
built into crantcli. To install it ahead of time, or from a script, run:

```bash
crantcli ask --download
```

A new crantcli release can bring a new model version. The next `ask` then
offers to download it and removes the old one.

## Options

```bash
# Only print the suggestion, for example to paste it elsewhere
crantcli ask --print "list the cell types in cell class EBc with counts"

# Run the suggestion without asking
crantcli ask --yes "edit history of 576460752669867599 as json"

# Fetch the table values again (they are cached for a day)
crantcli ask --refresh "show the ER4 neurons"
```

`ask` never runs a command unless you confirm it or pass `--yes`. Without a
terminal it only prints the suggestion.

## What it understands

raider knows every crantcli command and flag. It also sees the table values
your request mentions, so names like `PEN`, `ER2`, `kenyon cells`, or
`compass neurons` become the matching `--cell-type` or `--cell-class` values.
It can only write values that appear in your request or in the table, and
flags such as `--open`, `--count`, or `--intersect` only when the request asks
for them.

Requests work best when they name what you want and how to show it:

```bash
crantcli ask "get all ER cell class, label them and open on the browser"
crantcli ask "give cave history of 576460752669867599 label it and open on the browser"
crantcli ask "list cell types in cell class EBc for count"
```

The model runs locally. Only the table value lookup talks to SeaTable.
