package cmd

import (
	"encoding/json"
	"fmt"

	"crantinject/internal/nglstate"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Show info about a Neuroglancer state",
	Long: `Show layers, types, and segment counts for a Neuroglancer state.

Uses smart input: reads from --state flag, stdin, clipboard, or default template.

Examples:
  crantcli inspect              # reads from clipboard
  crantcli inspect -s state.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := nglstate.LoadState(inspectState, false)
		if err != nil {
			return err
		}

		fmt.Printf("Source: %s\n\n", result.Source)

		layersRaw, ok := result.State["layers"]
		if !ok {
			fmt.Println("No layers found")
			return nil
		}

		layers, ok := layersRaw.([]interface{})
		if !ok {
			fmt.Println("Invalid layers format")
			return nil
		}

		fmt.Printf("Layers (%d):\n", len(layers))
		for i, l := range layers {
			layer, ok := l.(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := layer["name"].(string)
			layerType, _ := layer["type"].(string)
			source := formatLayerSource(layer["source"])

			fmt.Printf("  [%d] %s (%s)\n", i, name, layerType)
			if source != "" {
				fmt.Printf("      source: %s\n", source)
			}

			if layerType == "segmentation" {
				segments := countSegments(layer)
				fmt.Printf("      segments: %d\n", segments)

				if colors, ok := layer["segmentColors"].(map[string]interface{}); ok {
					fmt.Printf("      colors: %d entries\n", len(colors))
				}
			}
		}

		return nil
	},
}

var inspectState string

func init() {
	inspectCmd.Flags().StringVarP(&inspectState, "state", "s", "", "Neuroglancer state (URL or file path)")
	rootCmd.AddCommand(inspectCmd)
}

func countSegments(layer map[string]interface{}) int {
	segsRaw, ok := layer["segments"]
	if !ok {
		return 0
	}
	segs, ok := segsRaw.([]interface{})
	if !ok {
		return 0
	}
	return len(segs)
}

func formatLayerSource(source interface{}) string {
	switch v := source.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
