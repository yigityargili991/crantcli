package seatable

import (
	"strings"
	"testing"
)

func TestQueryNeuronsPaginatesAndFiltersRegion(t *testing.T) {
	page1 := make([]map[string]interface{}, queryPageSize)
	for i := range page1 {
		page1[i] = neuronSQLRow("root-"+toString(float64(i)), []interface{}{"452098"})
	}
	page2 := []map[string]interface{}{
		neuronSQLRow("root-match", []interface{}{"452098", "645386"}),
		neuronSQLRow("root-miss", []interface{}{"333131"}),
	}

	var sqls []string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			sqls = append(sqls, sql)
			switch {
			case strings.Contains(sql, "OFFSET 0"):
				return &SQLResponse{Results: page1}, nil
			case strings.Contains(sql, "OFFSET 1000"):
				return &SQLResponse{Results: page2}, nil
			default:
				t.Fatalf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	rows, err := QueryNeurons(client, &Filters{Region: "LX"})
	if err != nil {
		t.Fatalf("QueryNeurons returned error: %v", err)
	}

	if len(sqls) != 2 {
		t.Fatalf("ExecuteSQL called %d times, want 2", len(sqls))
	}
	for _, sql := range sqls {
		if strings.Contains(sql, "`region` =") {
			t.Fatalf("expected region filtering to happen outside SQL, got %q", sql)
		}
	}

	if got, want := len(rows), queryPageSize+1; got != want {
		t.Fatalf("QueryNeurons returned %d rows, want %d", got, want)
	}
	if rows[len(rows)-1].Region != "LX, LW" {
		t.Fatalf("last row region = %q, want %q", rows[len(rows)-1].Region, "LX, LW")
	}
}

func TestQueryDistinctRegionCountsResolvedNames(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			return &SQLResponse{
				Results: []map[string]interface{}{
					{"region": []interface{}{"452098"}},
					{"region": []interface{}{"452098", "645386"}},
					{"region": []interface{}{"333131"}},
				},
			}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	resp, err := QueryDistinct(client, "region", &Filters{}, true)
	if err != nil {
		t.Fatalf("QueryDistinct returned error: %v", err)
	}

	if got, want := len(resp.Results), 3; got != want {
		t.Fatalf("len(resp.Results) = %d, want %d", got, want)
	}

	if got := resp.Results[0]["region"]; got != "LX" {
		t.Fatalf("first region = %v, want LX", got)
	}
	if got := resp.Results[0]["count"]; got != 2 {
		t.Fatalf("first count = %v, want 2", got)
	}
}

func TestResolveSelectFilterIDSupportsNamesAndIDs(t *testing.T) {
	idToName := map[string]string{"452098": "LX"}
	nameToID := map[string]string{"lx": "452098"}

	got, err := resolveSelectFilterID("LX", idToName, nameToID, "region")
	if err != nil {
		t.Fatalf("resolveSelectFilterID returned error: %v", err)
	}
	if got != "452098" {
		t.Fatalf("resolveSelectFilterID(LX) = %q, want %q", got, "452098")
	}

	got, err = resolveSelectFilterID("452098", idToName, nameToID, "region")
	if err != nil {
		t.Fatalf("resolveSelectFilterID returned error: %v", err)
	}
	if got != "452098" {
		t.Fatalf("resolveSelectFilterID(452098) = %q, want %q", got, "452098")
	}
}

func neuronSQLRow(rootID string, region []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"root_id":      rootID,
		"super_class":  "central",
		"cell_class":   "cx",
		"cell_type":    "EPG/PEG",
		"cell_subtype": "ER1",
		"side":         "left",
		"region":       region,
		"tract":        "tract",
		"nerve":        "nerve",
		"hemilineage":  "hemi",
		"proofread":    "true",
	}
}

func regionMetadata() *MetadataResponse {
	return &MetadataResponse{
		Metadata: struct {
			Tables []TableMeta `json:"tables"`
		}{
			Tables: []TableMeta{
				{
					Name: "CRANTb_meta",
					Columns: []ColumnDef{
						{
							Name: "region",
							Data: &ColumnData{
								Options: []SelectOption{
									{ID: "333131", Name: "CX"},
									{ID: "645386", Name: "LW"},
									{ID: "452098", Name: "LX"},
								},
							},
						},
					},
				},
			},
		},
	}
}
