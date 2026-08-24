package cmd

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

func TestCompleteListFieldsFiltersByPrefix(t *testing.T) {
	comps, directive := completeListFields(nil, nil, "cell_")

	want := []string{"cell_class", "cell_type", "cell_subtype", "cell_instance"}
	if !reflect.DeepEqual(comps, want) {
		t.Fatalf("completeListFields() = %v, want %v", comps, want)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
}

func TestCompleteListFieldsStopsAfterFirstArgument(t *testing.T) {
	comps, directive := completeListFields(nil, []string{"cell_type"}, "")

	if len(comps) != 0 {
		t.Fatalf("completeListFields() = %v, want no completions after first argument", comps)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
}

func TestCompleteStaticValuesFiltersCaseInsensitive(t *testing.T) {
	comps, directive := completeStaticValues(colorCompletions)(nil, nil, "TU")

	want := []string{"turquoise"}
	if !reflect.DeepEqual(comps, want) {
		t.Fatalf("completeStaticValues() = %v, want %v", comps, want)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
}

func TestCompleteColorByFields(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		want       []string
		wantAbsent []string
	}{
		{
			name:  "first field includes every field kind",
			input: "",
			want:  []string{"cell_type", "pos_x", "root_id", "group"},
		},
		{
			name:       "second field excludes the outer and outer-only fields",
			input:      "cell_type,",
			want:       []string{"cell_type,cell_subtype", "cell_type,root_id"},
			wantAbsent: []string{"cell_type,cell_type", "cell_type,pos_x", "cell_type,group"},
		},
		{
			name:  "second field filters by its prefix",
			input: "group,cell_",
			want:  []string{"group,cell_class", "group,cell_type", "group,cell_subtype", "group,cell_instance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, directive := completeColorByFields(nil, nil, tt.input)
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Fatalf("directive = %v, want NoFileComp", directive)
			}
			for _, want := range tt.want {
				if !slices.Contains(got, want) {
					t.Errorf("completions = %v, want %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if slices.Contains(got, absent) {
					t.Errorf("completions = %v, should exclude %q", got, absent)
				}
			}
		})
	}

	for _, input := range []string{"pos_x,", "cell_type,side,"} {
		got, directive := completeColorByFields(nil, nil, input)
		if len(got) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("completeColorByFields(%q) = (%v, %v), want no completions", input, got, directive)
		}
	}
}

func TestCompleteDistinctFieldUsesCurrentFilters(t *testing.T) {
	restore := replaceCompletionHooks(t)
	defer restore()

	var gotField string
	var gotFilters *seatable.Filters
	var gotWithCount bool

	completionClient = func() (*seatable.Client, error) {
		return &seatable.Client{}, nil
	}
	completionQueryDistinct = func(_ *seatable.Client, field string, filters *seatable.Filters, withCount bool) (*seatable.SQLResponse, error) {
		gotField = field
		gotFilters = filters
		gotWithCount = withCount
		return &seatable.SQLResponse{Results: []map[string]interface{}{
			{"cell_type": "EPG/PEG"},
			{"cell_type": "PFN"},
			{"cell_type": "EPG/PEG"},
			{"cell_type": ""},
		}}, nil
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("super-class", "", "")
	cmd.Flags().StringArray("cell-class", nil, "")
	cmd.Flags().String("region", "", "")

	if err := cmd.Flags().Set("super-class", "sensory"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("cell-class", "kenyon_cell"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("region", "LX"); err != nil {
		t.Fatal(err)
	}

	comps, directive := completeDistinctField("cell_type")(cmd, nil, "EP")

	if gotField != "cell_type" {
		t.Fatalf("field = %q, want cell_type", gotField)
	}
	if gotWithCount {
		t.Fatalf("withCount = true, want false")
	}
	if gotFilters == nil {
		t.Fatalf("filters were nil")
	}
	if gotFilters.SuperClass != "sensory" {
		t.Fatalf("SuperClass = %q, want sensory", gotFilters.SuperClass)
	}
	if gotFilters.CellClass != "kenyon_cell" {
		t.Fatalf("CellClass = %q, want kenyon_cell", gotFilters.CellClass)
	}
	if gotFilters.CellType != "" {
		t.Fatalf("CellType = %q, want empty while completing cell_type", gotFilters.CellType)
	}
	if gotFilters.Region != "LX" {
		t.Fatalf("Region = %q, want LX", gotFilters.Region)
	}

	want := []string{"EPG/PEG"}
	if !reflect.DeepEqual(comps, want) {
		t.Fatalf("completions = %v, want %v", comps, want)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
}

func TestCompleteDistinctValuesPrefersMetadataOptionsWithoutFilters(t *testing.T) {
	restore := replaceCompletionHooks(t)
	defer restore()

	var queryCalled bool
	completionClient = func() (*seatable.Client, error) {
		return &seatable.Client{}, nil
	}
	completionSelectOptions = func(_ *seatable.Client, field string) ([]string, error) {
		if field != "region" {
			t.Fatalf("field = %q, want region", field)
		}
		return []string{"LX", "MB", "AL"}, nil
	}
	completionQueryDistinct = func(_ *seatable.Client, _ string, _ *seatable.Filters, _ bool) (*seatable.SQLResponse, error) {
		queryCalled = true
		return nil, nil
	}

	comps, directive := completeDistinctValues("region", &seatable.Filters{}, "L")

	if queryCalled {
		t.Fatalf("completionQueryDistinct was called despite metadata options")
	}
	want := []string{"LX"}
	if !reflect.DeepEqual(comps, want) {
		t.Fatalf("completions = %v, want %v", comps, want)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
}

func TestCompletionFiltersUsesBundleAsRegionAlias(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("region", "", "")
	cmd.Flags().String("bundle", "", "")
	if err := cmd.Flags().Set("bundle", "LX"); err != nil {
		t.Fatal(err)
	}

	filters := completionFilters(cmd, "cell_type")
	if filters.Region != "LX" {
		t.Fatalf("Region = %q, want LX from --bundle", filters.Region)
	}
}

func TestCompleteDistinctValuesReturnsActiveHelpWhenUnavailable(t *testing.T) {
	restore := replaceCompletionHooks(t)
	defer restore()

	completionClient = func() (*seatable.Client, error) {
		return nil, errors.New("boom")
	}

	comps, directive := completeDistinctValues("cell_type", &seatable.Filters{}, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
	if len(comps) == 0 || !strings.Contains(comps[0], "data completions unavailable") {
		t.Fatalf("completions = %v, want active help about unavailable data completions", comps)
	}
}

func replaceCompletionHooks(t *testing.T) func() {
	t.Helper()

	oldClient := completionClient
	oldSelectOptions := completionSelectOptions
	oldQueryDistinct := completionQueryDistinct

	return func() {
		completionClient = oldClient
		completionSelectOptions = oldSelectOptions
		completionQueryDistinct = oldQueryDistinct
	}
}
