# crantcli add

Query neurons and inject root IDs into a Neuroglancer state

## Synopsis

Query the CRANT dataset for neurons matching the given filters and inject
their root IDs into a Neuroglancer state.

Smart input resolution (when no --state is given):

  1. Check stdin for piped JSON
  2. Check clipboard for a Neuroglancer URL (read implicitly)
  3. Fall back to the configured or built-in CRANT scene template

With no --output, the resulting Neuroglancer URL is written back to the
clipboard, overwriting its current contents.

```
crantcli add [flags]
```

## Examples

```bash
  # Smart: checks clipboard for a Neuroglancer URL, injects, and copies back
  crantcli add --cell-type ER

  # Explicit file I/O
  crantcli add --cell-type ER -s state.json -o modified.json

  # Generate a fresh state
  crantcli add --cell-type ER --generate

  # Open the updated state in a browser
  crantcli add --cell-type ER --open

  # Print root IDs without manipulating a state
  crantcli add --cell-type ER --root-ids-only

  # Add multiple cell types with per-group coloring
  crantcli add --cell-type ER --cell-type EPG/PEG --color colored

  # Add sensory neurons and color by cell_type
  crantcli add --super-class sensory --color-by cell_type

  # Show cell types next to root IDs in the Seg. panel (requires the gh CLI)
  crantcli add --cell-type ER --labels

  # Add all neurons annotated to bundle/region LX
  crantcli add --bundle LX

  # Stack classifiers as a union
  crantcli add --cell-class LNO --cell-subtype PFNc --cell-subtype PFNm3 --cell-type PEN --color-by cell_subtype

  # Intersect classifiers instead
  crantcli add --intersect --cell-class ER --cell-type ER

  # Sub-color by cell_subtype within each query group
  crantcli add --cell-type ER --color-sub --color blue
```

## Options

```
      --bundle stringArray         Filter by bundle region annotation (repeatable alias of --region, e.g. LX)
      --cell-class stringArray     Filter by cell_class (repeatable for multiple classes)
      --cell-subtype stringArray   Filter by cell_subtype (repeatable for multiple subtypes)
      --cell-type stringArray      Filter by cell_type (repeatable for multiple types)
      --color string               Segment color: named (blue, red, green, turquoise, orange, purple, yellow, pink, brown, indigo, teal, lime) with auto-toning, 'colored' for per-group palette cycling, or hex (#ff0000)
      --color-by string            Color matched rows by field: super_class, cell_class, cell_type, cell_subtype, cell_instance, column, side, region, tract, nerve, hemilineage, proofread
      --color-sub                  Sub-color neurons by cell_subtype within each query group
  -g, --generate                   Use the configured or built-in default state instead of the clipboard
  -h, --help                       help for add
      --intersect                  Intersect --cell-class/--cell-type/--cell-subtype as a cross-product (AND) instead of the default union (OR, each value its own group); rarely needed since these classifiers are hierarchical. Other filters (--super-class, --side, --region, ...) always apply to every group
      --labels                     Attach cell-type labels (via an ephemeral secret GitHub gist) so types show next to root IDs in the Seg. panel; requires the gh CLI, or a publish hook via --labels-hook/$CRANT_LABELS_HOOK
      --labels-hook string         Command to publish/clean label sources instead of a GitHub gist (receives info JSON on stdin, prints {"url","id"}); defaults to $CRANT_LABELS_HOOK
      --labels-ttl duration        Delete previously-created label sources older than this on each --labels run (default 168h0m0s)
  -l, --layer string               Target segmentation layer name
      --open                       Open updated Neuroglancer URL in default browser
  -o, --output string              Output file path (default: clipboard or stdout)
      --proofread string           Filter by proofread status
      --region stringArray         Filter by region (repeatable for multiple regions)
      --replace                    Replace existing segments instead of appending
      --root-ids-only              Print root IDs and copy them to the clipboard; no state manipulation
      --side string                Filter by side
  -s, --state string               Neuroglancer state (URL or file path)
      --super-class string         Filter by super_class
      --tract string               Filter by tract
```

## See also

* [crantcli](crantcli.md)	 - Query CRANT clonal raider ant connectome neurons and inject into Neuroglancer states

