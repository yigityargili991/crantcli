package cmd

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"crantinject/internal/seatable"

	"github.com/spf13/cobra"
)

var lookupColumnCmd = &cobra.Command{
	Use:         "lookup-column [root_id]",
	Short:       "Find the closest EPG/PEG neuron's column (region) by position",
	Long:        `Finds the closest EPG/PEG neuron to the given root ID (or position) by 3D Euclidean distance and prints its resolved region value plus the nearest EPG/PEG root ID.`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args:        cobra.MaximumNArgs(1),
}

func init() {
	var lookupColumnPos string

	lookupColumnCmd.Flags().StringVar(&lookupColumnPos, "pos", "", "Position as x,y,z (e.g. 100.5,200.3,300.1)")

	lookupColumnCmd.RunE = func(cmd *cobra.Command, args []string) error {
		var x, y, z float64
		hasPos := lookupColumnPos != ""

		if hasPos && len(args) > 0 {
			return fmt.Errorf("cannot combine root_id argument with --pos flag")
		}
		if !hasPos && len(args) == 0 {
			return fmt.Errorf("provide a root_id argument or use --pos x,y,z")
		}

		client, err := seatable.NewClient()
		if err != nil {
			return err
		}

		meta, err := client.FetchMetadata()
		if err != nil {
			return fmt.Errorf("fetching column metadata: %w", err)
		}
		regionOpts := seatable.SelectOptionMap(meta, "region")

		if hasPos {
			px, py, pz, err := parsePos(lookupColumnPos)
			if err != nil {
				return err
			}
			x, y, z = px, py, pz
		} else {
			rootID := args[0]
			neuron, err := seatable.QueryNeuronPosition(client, rootID, regionOpts)
			if err != nil {
				return err
			}
			if neuron == nil {
				return fmt.Errorf("no neuron found with root_id %q", rootID)
			}
			if !neuron.HasPosition() {
				return fmt.Errorf("neuron %s has no position coordinates", rootID)
			}
			x, y, z = neuron.X, neuron.Y, neuron.Z
		}

		candidates, err := seatable.QueryNeuronsWithPosition(client, regionOpts)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return fmt.Errorf("no EPG/PEG neurons found")
		}

		var closest *seatable.NeuronPositionRow
		closestDist := math.MaxFloat64

		for i := range candidates {
			c := &candidates[i]
			if !c.HasPosition() {
				continue
			}
			d := euclideanDistance(x, y, z, c.X, c.Y, c.Z)
			if d < closestDist {
				closestDist = d
				closest = c
			}
		}

		if closest == nil {
			return fmt.Errorf("no EPG/PEG neurons with valid position coordinates found")
		}

		fmt.Println(formatLookupColumnOutput(closest))
		return nil
	}

	rootCmd.AddCommand(lookupColumnCmd)
}

func formatLookupColumnOutput(neuron *seatable.NeuronPositionRow) string {
	return fmt.Sprintf("%s\t%s", neuron.Region, neuron.RootID)
}

func parsePos(s string) (float64, float64, float64, error) {
	posErr := fmt.Errorf("invalid --pos format; expected x,y,z (e.g. 100.5,200.3,300.1)")
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return 0, 0, 0, posErr
	}
	var coords [3]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, 0, 0, posErr
		}
		coords[i] = f
	}
	return coords[0], coords[1], coords[2], nil
}

func euclideanDistance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	dz := z1 - z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
