package cmd

import (
	"reflect"
	"testing"

	"crantcli/internal/seatable"
)

func TestResolveAddRegionFilter(t *testing.T) {
	t.Run("bundle aliases region", func(t *testing.T) {
		got, err := resolveAddRegionFilter("", "LX")
		if err != nil {
			t.Fatalf("resolveAddRegionFilter returned error: %v", err)
		}
		if got != "LX" {
			t.Fatalf("resolveAddRegionFilter = %q, want %q", got, "LX")
		}
	})

	t.Run("conflicting flags", func(t *testing.T) {
		_, err := resolveAddRegionFilter("CX", "LX")
		if err == nil {
			t.Fatal("expected conflict error when both region and bundle are set")
		}
	})
}

func TestResolveAddColorBy(t *testing.T) {
	t.Run("valid field", func(t *testing.T) {
		got, err := resolveAddColorBy("cell_type", false)
		if err != nil {
			t.Fatalf("resolveAddColorBy returned error: %v", err)
		}
		if got != "cell_type" {
			t.Fatalf("resolveAddColorBy = %q, want cell_type", got)
		}
	})

	t.Run("color-sub shorthand", func(t *testing.T) {
		got, err := resolveAddColorBy("", true)
		if err != nil {
			t.Fatalf("resolveAddColorBy returned error: %v", err)
		}
		if got != "cell_subtype" {
			t.Fatalf("resolveAddColorBy = %q, want cell_subtype", got)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		_, err := resolveAddColorBy("cell_type", true)
		if err == nil {
			t.Fatal("expected conflict error")
		}
	})

	t.Run("invalid field", func(t *testing.T) {
		_, err := resolveAddColorBy("not_a_field", false)
		if err == nil {
			t.Fatal("expected invalid field error")
		}
	})
}

func TestValidateAddInputs(t *testing.T) {
	err := validateAddInputs(&seatable.Filters{Region: "LX"}, false)
	if err != nil {
		t.Fatalf("validateAddInputs returned error: %v", err)
	}

	err = validateAddInputs(&seatable.Filters{}, false)
	if err == nil {
		t.Fatal("expected missing-filters error")
	}
}

func TestBuildColorByGroups(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "100", CellType: "ER", Side: "left"},
		{RootID: "200", CellType: "EPG/PEG", Side: "right"},
		{RootID: "", CellType: "ER", Side: "left"},
		{RootID: "300", CellType: "ER", Side: "right"},
		{RootID: "400", CellType: "", Side: "right"},
	}

	groups, labels := buildColorByGroups(rows, "cell_type")

	wantGroups := [][]string{{"100", "300"}, {"200"}, {"400"}}
	if !reflect.DeepEqual(groups, wantGroups) {
		t.Fatalf("groups = %#v, want %#v", groups, wantGroups)
	}

	wantLabels := []string{"cell_type=ER", "cell_type=EPG/PEG", "cell_type=(empty)"}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", labels, wantLabels)
	}
}

func TestAddColorByFieldValue_AllFields(t *testing.T) {
	row := seatable.NeuronRow{
		SuperClass:  "super",
		CellClass:   "class",
		CellType:    "type",
		CellSubtype: "subtype",
		Side:        "side",
		Region:      "region",
		Tract:       "tract",
		Nerve:       "nerve",
		Hemilineage: "hemilineage",
		Proofread:   "proofread",
	}

	tests := []struct {
		field string
		want  string
	}{
		{"super_class", "super"},
		{"cell_class", "class"},
		{"cell_type", "type"},
		{"cell_subtype", "subtype"},
		{"side", "side"},
		{"region", "region"},
		{"tract", "tract"},
		{"nerve", "nerve"},
		{"hemilineage", "hemilineage"},
		{"proofread", "proofread"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if got := addColorByFieldValue(row, tt.field); got != tt.want {
				t.Fatalf("addColorByFieldValue(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestBuildQuerySpecs_CrossProduct(t *testing.T) {
	base := &seatable.Filters{Side: "left"}

	t.Run("both class and type: cross-product", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"kenyon_cell"}, []string{"ER", "EPG/PEG"})
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellClass != "kenyon_cell" || specs[0].filters.CellType != "ER" {
			t.Errorf("spec[0] = class=%q type=%q, want kenyon_cell/ER", specs[0].filters.CellClass, specs[0].filters.CellType)
		}
		if specs[1].filters.CellClass != "kenyon_cell" || specs[1].filters.CellType != "EPG/PEG" {
			t.Errorf("spec[1] = class=%q type=%q, want kenyon_cell/EPG/PEG", specs[1].filters.CellClass, specs[1].filters.CellType)
		}
		// Base filter preserved
		if specs[0].filters.Side != "left" {
			t.Errorf("base filter Side lost, got %q", specs[0].filters.Side)
		}
	})

	t.Run("multi class x multi type", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"A", "B"}, []string{"X", "Y"})
		if len(specs) != 4 {
			t.Fatalf("got %d specs, want 4", len(specs))
		}
		labels := []string{specs[0].label, specs[1].label, specs[2].label, specs[3].label}
		want := []string{"A/X", "A/Y", "B/X", "B/Y"}
		if !reflect.DeepEqual(labels, want) {
			t.Errorf("labels = %v, want %v", labels, want)
		}
	})

	t.Run("only classes: independent groups", func(t *testing.T) {
		specs := buildQuerySpecs(base, []string{"kenyon_cell", "olfactory"}, nil)
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellClass != "kenyon_cell" || specs[0].filters.CellType != "" {
			t.Errorf("spec[0] = class=%q type=%q, want kenyon_cell/empty", specs[0].filters.CellClass, specs[0].filters.CellType)
		}
		if specs[1].filters.CellClass != "olfactory" || specs[1].filters.CellType != "" {
			t.Errorf("spec[1] = class=%q type=%q, want olfactory/empty", specs[1].filters.CellClass, specs[1].filters.CellType)
		}
	})

	t.Run("only types: independent groups", func(t *testing.T) {
		specs := buildQuerySpecs(base, nil, []string{"ER", "EPG/PEG"})
		if len(specs) != 2 {
			t.Fatalf("got %d specs, want 2", len(specs))
		}
		if specs[0].filters.CellType != "ER" || specs[0].filters.CellClass != "" {
			t.Errorf("spec[0] = class=%q type=%q, want empty/ER", specs[0].filters.CellClass, specs[0].filters.CellType)
		}
	})

	t.Run("neither: single group with base", func(t *testing.T) {
		specs := buildQuerySpecs(base, nil, nil)
		if len(specs) != 1 {
			t.Fatalf("got %d specs, want 1", len(specs))
		}
		if specs[0].filters.Side != "left" {
			t.Errorf("base filter Side lost, got %q", specs[0].filters.Side)
		}
		if specs[0].label != "" {
			t.Errorf("label should be empty, got %q", specs[0].label)
		}
	})
}

// TestBuildQuerySpecs_BaseFiltersPreservedInCrossProduct verifies that ALL base filter
// fields are preserved in every cross-product spec, not just Side.
func TestBuildQuerySpecs_BaseFiltersPreservedInCrossProduct(t *testing.T) {
	base := &seatable.Filters{
		SuperClass:  "super1",
		CellSubtype: "sub1",
		Side:        "left",
		Region:      "LX",
		Tract:       "tract1",
		Proofread:   "yes",
	}
	specs := buildQuerySpecs(base, []string{"classA", "classB"}, []string{"typeX"})
	if len(specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(specs))
	}
	for _, s := range specs {
		f := s.filters
		if f.SuperClass != "super1" {
			t.Errorf("spec %q: SuperClass=%q, want super1", s.label, f.SuperClass)
		}
		if f.CellSubtype != "sub1" {
			t.Errorf("spec %q: CellSubtype=%q, want sub1", s.label, f.CellSubtype)
		}
		if f.Side != "left" {
			t.Errorf("spec %q: Side=%q, want left", s.label, f.Side)
		}
		if f.Region != "LX" {
			t.Errorf("spec %q: Region=%q, want LX", s.label, f.Region)
		}
		if f.Tract != "tract1" {
			t.Errorf("spec %q: Tract=%q, want tract1", s.label, f.Tract)
		}
		if f.Proofread != "yes" {
			t.Errorf("spec %q: Proofread=%q, want yes", s.label, f.Proofread)
		}
	}
}

// TestBuildQuerySpecs_CrossProductDoesNotMutateBase verifies that buildQuerySpecs
// does not mutate the original base Filters struct.
func TestBuildQuerySpecs_CrossProductDoesNotMutateBase(t *testing.T) {
	base := &seatable.Filters{Side: "right"}
	original := *base

	buildQuerySpecs(base, []string{"classA"}, []string{"typeX", "typeY"})

	if *base != original {
		t.Errorf("buildQuerySpecs mutated base: got %+v, want %+v", *base, original)
	}
}

// TestBuildQuerySpecs_CrossProductLabels verifies the label format for cross-product.
func TestBuildQuerySpecs_CrossProductLabels(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, []string{"classA"}, []string{"typeX", "typeY"})
	if len(specs) != 2 {
		t.Fatalf("got %d specs, want 2", len(specs))
	}
	if specs[0].label != "classA/typeX" {
		t.Errorf("spec[0].label = %q, want classA/typeX", specs[0].label)
	}
	if specs[1].label != "classA/typeY" {
		t.Errorf("spec[1].label = %q, want classA/typeY", specs[1].label)
	}
}

// TestBuildQuerySpecs_ClassOnlyLabelIsClassName verifies single-dimension (class-only)
// labels match the class name.
func TestBuildQuerySpecs_ClassOnlyLabelIsClassName(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, []string{"kenyon_cell", "olfactory"}, nil)
	if specs[0].label != "kenyon_cell" {
		t.Errorf("spec[0].label = %q, want kenyon_cell", specs[0].label)
	}
	if specs[1].label != "olfactory" {
		t.Errorf("spec[1].label = %q, want olfactory", specs[1].label)
	}
}

// TestBuildQuerySpecs_TypeOnlyLabelIsTypeName verifies single-dimension (type-only)
// labels match the type name.
func TestBuildQuerySpecs_TypeOnlyLabelIsTypeName(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, nil, []string{"ER", "EPG/PEG"})
	if specs[0].label != "ER" {
		t.Errorf("spec[0].label = %q, want ER", specs[0].label)
	}
	if specs[1].label != "EPG/PEG" {
		t.Errorf("spec[1].label = %q, want EPG/PEG", specs[1].label)
	}
}

// TestBuildQuerySpecs_EmptySlicesEquivalentToNil verifies that empty slices []string{}
// behave identically to nil for both cellClasses and cellTypes.
func TestBuildQuerySpecs_EmptySlicesEquivalentToNil(t *testing.T) {
	base := &seatable.Filters{Side: "left"}

	specsNil := buildQuerySpecs(base, nil, nil)
	specsEmpty := buildQuerySpecs(base, []string{}, []string{})

	if len(specsNil) != len(specsEmpty) {
		t.Fatalf("nil vs empty: got %d vs %d specs", len(specsNil), len(specsEmpty))
	}
	if len(specsNil) != 1 {
		t.Fatalf("expected 1 spec for no-dimensions case, got %d", len(specsNil))
	}
	if specsNil[0].filters.Side != specsEmpty[0].filters.Side {
		t.Errorf("nil vs empty: Side mismatch")
	}
	if specsNil[0].label != "" || specsEmpty[0].label != "" {
		t.Errorf("nil vs empty: expected empty labels, got %q and %q", specsNil[0].label, specsEmpty[0].label)
	}
}

// TestBuildQuerySpecs_CrossProduct_ClassAndTypeNotLeakedToOtherSpecs verifies that
// CellClass assigned to spec[i] is not visible in spec[j] (no shared pointer).
func TestBuildQuerySpecs_CrossProduct_ClassAndTypeNotLeakedToOtherSpecs(t *testing.T) {
	base := &seatable.Filters{}
	specs := buildQuerySpecs(base, []string{"A", "B"}, []string{"X", "Y"})
	if len(specs) != 4 {
		t.Fatalf("got %d specs, want 4", len(specs))
	}
	// Mutate one spec's filters and verify others are not affected
	specs[0].filters.Side = "MODIFIED"
	for i := 1; i < len(specs); i++ {
		if specs[i].filters.Side == "MODIFIED" {
			t.Errorf("spec[%d] was affected by mutation of spec[0] — filters are shared (pointer leak)", i)
		}
	}
}

// TestExtractRootIDs_AllEmpty verifies that a slice of rows with no root IDs returns empty.
func TestExtractRootIDs_AllEmpty(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "", CellSubtype: "alpha"},
		{RootID: "", CellSubtype: "beta"},
	}
	ids := extractRootIDs(rows)
	if len(ids) != 0 {
		t.Errorf("expected empty ids, got %v", ids)
	}
}

// TestExtractRootIDsWithSubtype_AllEmpty verifies the same for extractRootIDsWithSubtype.
func TestExtractRootIDsWithSubtype_AllEmpty(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "", CellSubtype: "alpha"},
	}
	ids, sm := extractRootIDsWithSubtype(rows)
	if len(ids) != 0 {
		t.Errorf("expected empty ids, got %v", ids)
	}
	if len(sm) != 0 {
		t.Errorf("expected empty subtypeMap, got %v", sm)
	}
}

// TestExtractRootIDsWithSubtype_NilRows verifies no panic on nil input.
func TestExtractRootIDsWithSubtype_NilRows(t *testing.T) {
	ids, sm := extractRootIDsWithSubtype(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty ids for nil rows, got %v", ids)
	}
	if len(sm) != 0 {
		t.Errorf("expected empty map for nil rows, got %v", sm)
	}
}

// TestResolveAddRegionFilter_EdgeCases covers trimming and empty combinations.
func TestResolveAddRegionFilter_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		bundle  string
		want    string
		wantErr bool
	}{
		{"both empty", "", "", "", false},
		{"region only", "CX", "", "CX", false},
		{"bundle only", "", "LX", "LX", false},
		{"both set", "CX", "LX", "", true},
		{"region whitespace only", "  ", "", "", false},
		{"bundle whitespace only", "", "  ", "", false},
		{"region with spaces trimmed", "  CX  ", "", "CX", false},
		{"bundle with spaces trimmed", "", "  LX  ", "LX", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAddRegionFilter(tt.region, tt.bundle)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractRootIDsWithSubtype(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "100", CellSubtype: "alpha"},
		{RootID: "200", CellSubtype: "beta"},
		{RootID: "", CellSubtype: "gamma"}, // no root ID, should be skipped
		{RootID: "300", CellSubtype: ""},   // no subtype
	}

	ids, sm := extractRootIDsWithSubtype(rows)

	wantIDs := []string{"100", "200", "300"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("ids = %v, want %v", ids, wantIDs)
	}

	if sm["100"] != "alpha" {
		t.Errorf("sm[100] = %q, want alpha", sm["100"])
	}
	if sm["200"] != "beta" {
		t.Errorf("sm[200] = %q, want beta", sm["200"])
	}
	if sm["300"] != "" {
		t.Errorf("sm[300] = %q, want empty", sm["300"])
	}
	if _, ok := sm[""]; ok {
		t.Error("empty root ID should not be in subtypeMap")
	}
}
