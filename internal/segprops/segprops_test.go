package segprops

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"crantcli/internal/seatable"
)

// decoded mirrors the neuroglancer_segment_properties info shape for assertions.
type decoded struct {
	Type   string `json:"@type"`
	Inline struct {
		IDs        []string          `json:"ids"`
		Properties []json.RawMessage `json:"properties"`
	} `json:"inline"`
}

func parse(t *testing.T, data []byte) decoded {
	t.Helper()
	var d decoded
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	return d
}

func labelValues(t *testing.T, props []json.RawMessage) []string {
	t.Helper()
	for _, p := range props {
		var m struct {
			ID, Type string
			Values   []string
		}
		if err := json.Unmarshal(p, &m); err == nil && m.Type == "label" {
			return m.Values
		}
	}
	t.Fatal("no label property found")
	return nil
}

func TestBuildSegmentProperties_LabelOnly(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "200", CellType: "ER"},
		{RootID: "100", CellType: "EPG/PEG"},
	}
	data, err := BuildSegmentProperties(rows, Options{LabelField: "cell_type"})
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, data)
	if d.Type != typeName {
		t.Errorf("@type = %q, want %q", d.Type, typeName)
	}
	// Deterministic order: same length -> lexical, so "100" before "200".
	if !reflect.DeepEqual(d.Inline.IDs, []string{"100", "200"}) {
		t.Errorf("ids = %v, want [100 200]", d.Inline.IDs)
	}
	if got := labelValues(t, d.Inline.Properties); !reflect.DeepEqual(got, []string{"EPG/PEG", "ER"}) {
		t.Errorf("labels = %v, want [EPG/PEG ER]", got)
	}
}

func TestBuildSegmentProperties_EmptyCellTypeAndRootID(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "", CellType: "X"}, // skipped: empty root id
		{RootID: "1", CellType: ""}, // kept with empty label
		{RootID: "2", CellType: "DA1"},
	}
	data, err := BuildSegmentProperties(rows, Options{LabelField: "cell_type"})
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, data)
	if !reflect.DeepEqual(d.Inline.IDs, []string{"1", "2"}) {
		t.Errorf("ids = %v, want [1 2]", d.Inline.IDs)
	}
	if got := labelValues(t, d.Inline.Properties); !reflect.DeepEqual(got, []string{"", "DA1"}) {
		t.Errorf("labels = %v, want [\"\" DA1]", got)
	}
}

func TestBuildSegmentProperties_LabelFallback(t *testing.T) {
	// Empty cell_type falls back to cell_class (e.g. the MeMe case).
	rows := []seatable.NeuronRow{
		{RootID: "1", CellType: "", CellClass: "MeMe"},
		{RootID: "2", CellType: "ER", CellClass: "central"},
	}
	data, err := BuildSegmentProperties(rows, Options{
		LabelField:     "cell_type",
		LabelFallbacks: []string{"cell_class"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, data)
	if got := labelValues(t, d.Inline.Properties); !reflect.DeepEqual(got, []string{"MeMe", "ER"}) {
		t.Errorf("labels = %v, want [MeMe ER]", got)
	}
}

func TestBuildSegmentProperties_DeduplicatesRootID(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "5", CellType: "first"},
		{RootID: "5", CellType: "second"},
	}
	data, err := BuildSegmentProperties(rows, Options{LabelField: "cell_type"})
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, data)
	if len(d.Inline.IDs) != 1 {
		t.Fatalf("ids = %v, want one entry", d.Inline.IDs)
	}
	if got := labelValues(t, d.Inline.Properties); got[0] != "first" {
		t.Errorf("label = %q, want first (first-wins)", got[0])
	}
}

func TestBuildSegmentProperties_DeterministicOrder(t *testing.T) {
	a := []seatable.NeuronRow{
		{RootID: "30", CellType: "c"},
		{RootID: "10", CellType: "a"},
		{RootID: "20", CellType: "b"},
	}
	b := []seatable.NeuronRow{
		{RootID: "20", CellType: "b"},
		{RootID: "30", CellType: "c"},
		{RootID: "10", CellType: "a"},
	}
	da, err := BuildSegmentProperties(a, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	db, err := BuildSegmentProperties(b, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(da, db) {
		t.Error("output is not stable across input orderings")
	}
}

func TestBuildSegmentProperties_Tags(t *testing.T) {
	rows := []seatable.NeuronRow{
		{RootID: "1", CellType: "ER", CellClass: "central brain", Side: "left", SuperClass: "sensory"},
		{RootID: "2", CellType: "PB", CellClass: "central brain", Side: "right"},
	}
	data, err := BuildSegmentProperties(rows, Options{
		LabelField: "cell_type",
		TagFields:  []string{"cell_class", "side", "super_class"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, data)

	var tags struct {
		ID, Type string
		Tags     []string
		Values   [][]int
	}
	found := false
	for _, p := range d.Inline.Properties {
		var m struct {
			ID, Type string
			Tags     []string
			Values   [][]int
		}
		if err := json.Unmarshal(p, &m); err == nil && m.Type == "tags" {
			tags = m
			found = true
		}
	}
	if !found {
		t.Fatal("no tags property found")
	}

	// Tags are sanitized (spaces -> _), prefixed by field, and globally sorted.
	wantTags := []string{"class_central_brain", "side_left", "side_right", "super_sensory"}
	if !reflect.DeepEqual(tags.Tags, wantTags) {
		t.Errorf("tags = %v, want %v", tags.Tags, wantTags)
	}
	// Row 1: class_central_brain(0), side_left(1), super_sensory(3) -> ascending.
	if !reflect.DeepEqual(tags.Values[0], []int{0, 1, 3}) {
		t.Errorf("row 1 indices = %v, want [0 1 3]", tags.Values[0])
	}
	// Row 2: class_central_brain(0), side_right(2); no super_class.
	if !reflect.DeepEqual(tags.Values[1], []int{0, 2}) {
		t.Errorf("row 2 indices = %v, want [0 2]", tags.Values[1])
	}
}

func TestBuildSegmentProperties_NoTagsWhenAllEmpty(t *testing.T) {
	rows := []seatable.NeuronRow{{RootID: "1", CellType: "ER"}}
	data, err := BuildSegmentProperties(rows, Options{LabelField: "cell_type", TagFields: []string{"cell_class"}})
	if err != nil {
		t.Fatal(err)
	}
	d := parse(t, data)
	if len(d.Inline.Properties) != 1 {
		t.Errorf("expected only the label property, got %d", len(d.Inline.Properties))
	}
}

func TestSanitizeTag(t *testing.T) {
	tests := []struct {
		prefix, value, want string
	}{
		{"class", "Central Brain", "class_central_brain"},
		{"type", "EPG/PEG", "type_epg_peg"},
		{"side", "#left", "side_left"},
		{"class", "  ", ""},
		{"", "Plain", "plain"},
	}
	for _, tt := range tests {
		if got := sanitizeTag(tt.prefix, tt.value); got != tt.want {
			t.Errorf("sanitizeTag(%q,%q) = %q, want %q", tt.prefix, tt.value, got, tt.want)
		}
	}
}
