# Get started

`crantcli` connects three parts of a connectomics workflow:

1. **CRANT metadata** supplies neuron classifications and root IDs.
2. **Neuroglancer states** hold the scene you want to inspect.
3. **CAVE** tells you whether a stored root ID is still current after proofreading.

## The shortest path

```bash
# 1. After installing crantcli, store your SeaTable and CAVE tokens
crantcli setup

# 2. Start from the built-in scene and open the result
crantcli add --cell-type ER --generate --open
```

The `add` command queries CRANT, adds matching root IDs to the segmentation layer, copies the updated Neuroglancer URL to the clipboard, and opens it in your browser.

## Choose the next step

- [Install](install.md) on macOS, Linux, or Windows.
- [Configure access](configure.md) with interactive setup or environment variables.
- [Build your first scene](first-scene.md) and learn the clipboard workflow.

!!! tip "No clipboard required"
    Every state-editing workflow can use explicit files. Pass `--state input.json --output result.json` when you want reproducible, scriptable I/O.
