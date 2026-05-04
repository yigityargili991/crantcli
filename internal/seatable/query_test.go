package seatable

import (
	"fmt"
	"strconv"
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

func TestExecutePagedSelectAllowsExactSafetyLimit(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			offset := parseOffset(t, sql)
			switch {
			case offset < maxPagedRows:
				return &SQLResponse{Results: pagedSelectRows(queryPageSize)}, nil
			case offset == maxPagedRows:
				return &SQLResponse{Results: nil}, nil
			default:
				t.Fatalf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}

	rows, err := executePagedSelect(client, "`root_id`", "")
	if err != nil {
		t.Fatalf("executePagedSelect returned error at safety limit: %v", err)
	}
	if got, want := len(rows), maxPagedRows; got != want {
		t.Fatalf("len(rows) = %d, want %d", got, want)
	}
}

func TestExecutePagedSelectErrorsWhenResultSetExceedsSafetyLimit(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			offset := parseOffset(t, sql)
			switch {
			case offset < maxPagedRows:
				return &SQLResponse{Results: pagedSelectRows(queryPageSize)}, nil
			case offset == maxPagedRows:
				return &SQLResponse{Results: pagedSelectRows(1)}, nil
			default:
				t.Fatalf("unexpected SQL: %s", sql)
				return nil, nil
			}
		},
	}

	rows, err := executePagedSelect(client, "`root_id`", "")
	if err == nil {
		t.Fatalf("executePagedSelect returned nil error for oversized result set; got %d rows", len(rows))
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("query exceeded %d row safety limit", maxPagedRows)) {
		t.Fatalf("executePagedSelect error = %q, want safety limit message", err.Error())
	}
}

func TestQueryNeuronPositionReturnsRowWhenPositionIsMalformed(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			return &SQLResponse{
				Results: []map[string]interface{}{
					{
						"root_id":   "root-malformed",
						"region":    []interface{}{"452098"},
						"cell_type": "EPG/PEG",
						"position":  nil,
					},
				},
			}, nil
		},
	}

	row, err := QueryNeuronPosition(client, "root-malformed", map[string]string{"452098": "LX"})
	if err != nil {
		t.Fatalf("QueryNeuronPosition returned error: %v", err)
	}
	if row == nil {
		t.Fatalf("QueryNeuronPosition returned nil row for an existing neuron with malformed position")
	}
	if row.RootID != "root-malformed" {
		t.Fatalf("QueryNeuronPosition returned root_id %q, want %q", row.RootID, "root-malformed")
	}
	if row.HasPosition() {
		t.Fatalf("QueryNeuronPosition returned HasPosition()=true for malformed position")
	}
}

func TestQueryNeuronPositionsPreservesMalformedTargets(t *testing.T) {
	var gotSQL string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			gotSQL = sql
			return &SQLResponse{
				Results: []map[string]interface{}{
					{
						"root_id":   "target-malformed",
						"region":    []interface{}{"452098"},
						"cell_type": "PFN",
						"side":      "left",
						"position":  nil,
					},
				},
			}, nil
		},
	}

	rows, err := QueryNeuronPositions(client, &Filters{CellType: "PFN"}, map[string]string{"452098": "LX"})
	if err != nil {
		t.Fatalf("QueryNeuronPositions returned error: %v", err)
	}
	if got, want := len(rows), 1; got != want {
		t.Fatalf("len(rows) = %d, want %d", got, want)
	}
	if !strings.Contains(gotSQL, "`side`") {
		t.Fatalf("QueryNeuronPositions SQL omitted side column: %s", gotSQL)
	}
	if rows[0].RootID != "target-malformed" {
		t.Fatalf("RootID = %q, want target-malformed", rows[0].RootID)
	}
	if rows[0].Side != "left" {
		t.Fatalf("Side = %q, want left", rows[0].Side)
	}
	if rows[0].HasPosition() {
		t.Fatalf("HasPosition() = true, want false for malformed position")
	}
}

func TestQueryNeuronsWithPositionIncludesSideAndSkipsMalformedRows(t *testing.T) {
	var gotSQL string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			gotSQL = sql
			return &SQLResponse{
				Results: []map[string]interface{}{
					{
						"root_id":   "valid-epg",
						"region":    []interface{}{"452098"},
						"cell_type": "EPG/PEG",
						"side":      "right",
						"position":  "1,2,3",
					},
					{
						"root_id":   "bad-epg",
						"region":    []interface{}{"452098"},
						"cell_type": "EPG/PEG",
						"side":      "left",
						"position":  nil,
					},
				},
			}, nil
		},
	}

	rows, err := QueryNeuronsWithPosition(client, map[string]string{"452098": "LX"})
	if err != nil {
		t.Fatalf("QueryNeuronsWithPosition returned error: %v", err)
	}
	if got, want := len(rows), 1; got != want {
		t.Fatalf("len(rows) = %d, want %d", got, want)
	}
	if !strings.Contains(gotSQL, "`side`") {
		t.Fatalf("QueryNeuronsWithPosition SQL omitted side column: %s", gotSQL)
	}
	if rows[0].RootID != "valid-epg" {
		t.Fatalf("RootID = %q, want valid-epg", rows[0].RootID)
	}
	if rows[0].Side != "right" {
		t.Fatalf("Side = %q, want right", rows[0].Side)
	}
	if !rows[0].HasPosition() {
		t.Fatalf("HasPosition() = false, want true")
	}
}

func TestParsePositionValueRejectsMalformedArrays(t *testing.T) {
	tests := []struct {
		name string
		pos  []interface{}
	}{
		{name: "nil elements", pos: []interface{}{nil, nil, nil}},
		{name: "empty strings", pos: []interface{}{"", "", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, z, err := parsePositionValue(tt.pos)
			if err == nil {
				t.Fatalf("parsePositionValue(%v) returned nil error for malformed array", tt.pos)
			}
			if x != 0 || y != 0 || z != 0 {
				t.Fatalf("parsePositionValue(%v) = (%v, %v, %v), want zeros for rejected input", tt.pos, x, y, z)
			}
		})
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

func pagedSelectRows(n int) []map[string]interface{} {
	rows := make([]map[string]interface{}, n)
	for i := range rows {
		rows[i] = map[string]interface{}{"root_id": "root-" + strconv.Itoa(i)}
	}
	return rows
}

func parseOffset(t *testing.T, sql string) int {
	t.Helper()

	idx := strings.LastIndex(sql, "OFFSET ")
	if idx == -1 {
		t.Fatalf("SQL missing OFFSET clause: %s", sql)
	}

	offset, err := strconv.Atoi(strings.TrimSpace(sql[idx+len("OFFSET "):]))
	if err != nil {
		t.Fatalf("parsing OFFSET from %q: %v", sql, err)
	}
	return offset
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
