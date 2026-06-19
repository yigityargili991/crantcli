package cmd

import (
	"fmt"
	"io"

	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list <field>",
	Short: "List distinct values for a classification field",
	Long: `List distinct values for a classification field from the CRANT dataset.

Valid fields: super_class, cell_class, cell_type, cell_subtype, cell_instance, side, region, tract, nerve, hemilineage, proofread

Examples:
  crantcli list super_class --count
  crantcli list cell_class --super-class sensory --count
  crantcli list cell_type --cell-class kenyon_cell`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args:        cobra.ExactArgs(1),
}

func init() {
	var (
		listCount       bool
		listSuperClass  string
		listCellClass   string
		listCellType    string
		listCellSubtype string
		listSide        string
		listRegion      string
		listTract       string
	)

	listCmd.Flags().BoolVar(&listCount, "count", false, "Show count of neurons for each value")
	listCmd.Flags().StringVar(&listSuperClass, "super-class", "", "Filter by super_class")
	listCmd.Flags().StringVar(&listCellClass, "cell-class", "", "Filter by cell_class")
	listCmd.Flags().StringVar(&listCellType, "cell-type", "", "Filter by cell_type")
	listCmd.Flags().StringVar(&listCellSubtype, "cell-subtype", "", "Filter by cell_subtype")
	listCmd.Flags().StringVar(&listSide, "side", "", "Filter by side")
	listCmd.Flags().StringVar(&listRegion, "region", "", "Filter by region")
	listCmd.Flags().StringVar(&listTract, "tract", "", "Filter by tract")
	listCmd.ValidArgsFunction = completeListFields
	registerClassificationFlagCompletions(listCmd,
		"super-class",
		"cell-class",
		"cell-type",
		"cell-subtype",
		"side",
		"region",
		"tract",
	)

	listCmd.RunE = func(cmd *cobra.Command, args []string) error {
		field := args[0]

		client, err := seatable.NewClient()
		if err != nil {
			return err
		}

		filters := &seatable.Filters{
			SuperClass:  listSuperClass,
			CellClass:   listCellClass,
			CellType:    listCellType,
			CellSubtype: listCellSubtype,
			Side:        listSide,
			Region:      listRegion,
			Tract:       listTract,
		}

		resp, err := seatable.QueryDistinct(client, field, filters, listCount)
		if err != nil {
			return err
		}

		return writeDistinctResults(cmd.OutOrStdout(), field, resp, listCount)
	}

	rootCmd.AddCommand(listCmd)
}

func writeDistinctResults(w io.Writer, field string, resp *seatable.SQLResponse, withCount bool) error {
	for _, row := range resp.Results {
		val := row[field]
		if val == nil || val == "" {
			continue
		}
		if withCount {
			if _, err := fmt.Fprintf(w, "%-40v %v\n", val, row["count"]); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintln(w, val); err != nil {
			return err
		}
	}
	return nil
}
