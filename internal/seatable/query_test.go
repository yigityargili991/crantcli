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
	if rows[len(rows)-1].CellInstance != "instance-root-match" {
		t.Fatalf("last row cell_instance = %q, want instance-root-match", rows[len(rows)-1].CellInstance)
	}
}

func TestQueryNeuronsFiltersMultipleRegions(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			return &SQLResponse{Results: []map[string]interface{}{
				neuronSQLRow("root-lx", []interface{}{"452098"}),
				neuronSQLRow("root-lw", []interface{}{"645386"}),
				neuronSQLRow("root-miss", []interface{}{"333131"}),
			}}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	rows, err := QueryNeurons(client, &Filters{Regions: []string{"LX", "LW"}})
	if err != nil {
		t.Fatalf("QueryNeurons returned error: %v", err)
	}

	if got, want := len(rows), 2; got != want {
		t.Fatalf("QueryNeurons returned %d rows, want %d", got, want)
	}
	if rows[0].RootID != "root-lx" || rows[1].RootID != "root-lw" {
		t.Fatalf("root IDs = %q, %q; want root-lx, root-lw", rows[0].RootID, rows[1].RootID)
	}
	if got, want := strings.Join(rows[0].MatchedRegions, ","), "LX"; got != want {
		t.Fatalf("first row MatchedRegions = %q, want %q", got, want)
	}
	if got, want := strings.Join(rows[1].MatchedRegions, ","), "LW"; got != want {
		t.Fatalf("second row MatchedRegions = %q, want %q", got, want)
	}
}

func TestQueryNeuronsByRootIDsFiltersExactIDs(t *testing.T) {
	var gotSQL string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			gotSQL = sql
			return &SQLResponse{Results: []map[string]interface{}{
				neuronSQLRow("222", []interface{}{"452098"}),
			}}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	rows, err := QueryNeuronsByRootIDs(client, []string{" 111 ", "222", "111", "3'3"})
	if err != nil {
		t.Fatalf("QueryNeuronsByRootIDs returned error: %v", err)
	}

	wantWhere := "`root_id` IN ('111', '222', '3''3')"
	if !strings.Contains(gotSQL, wantWhere) {
		t.Fatalf("SQL = %q, want exact root ID filter %q", gotSQL, wantWhere)
	}
	if len(rows) != 1 || rows[0].RootID != "222" || rows[0].CellType != "EPG/PEG" {
		t.Fatalf("rows = %#v, want metadata for root ID 222", rows)
	}
}

func TestQueryNeuronsByRootIDsEmptySkipsQuery(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			t.Fatalf("unexpected SQL query: %s", sql)
			return nil, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			t.Fatal("unexpected metadata query")
			return nil, nil
		},
	}

	rows, err := QueryNeuronsByRootIDs(client, []string{"", " "})
	if err != nil {
		t.Fatalf("QueryNeuronsByRootIDs returned error: %v", err)
	}
	if rows != nil {
		t.Fatalf("rows = %#v, want nil", rows)
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

func TestQueryDistinctSideResolvesOptionNamesWithCount(t *testing.T) {
	var gotSQL string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			gotSQL = sql
			// SeaTable groups by the stored single-select option IDs.
			return &SQLResponse{
				Results: []map[string]interface{}{
					{"side": "553927", "count": float64(120)},
					{"side": "884118", "count": float64(50)},
				},
			}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	resp, err := QueryDistinct(client, "side", &Filters{}, true)
	if err != nil {
		t.Fatalf("QueryDistinct returned error: %v", err)
	}

	if strings.Contains(gotSQL, "OFFSET") {
		t.Fatalf("expected single GROUP BY query for side, got paged select: %q", gotSQL)
	}
	if got, want := len(resp.Results), 2; got != want {
		t.Fatalf("len(resp.Results) = %d, want %d", got, want)
	}
	if got := resp.Results[0]["side"]; got != "left" {
		t.Fatalf("first side = %v, want left", got)
	}
	if got := resp.Results[0]["count"]; got != 120 {
		t.Fatalf("first count = %v, want 120", got)
	}
	if got := resp.Results[1]["side"]; got != "right" {
		t.Fatalf("second side = %v, want right", got)
	}
	if got := resp.Results[1]["count"]; got != 50 {
		t.Fatalf("second count = %v, want 50", got)
	}
}

func TestQueryDistinctSideResolvesOptionNamesSortedByName(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			// Returned in option-ID order; resolved names must sort alphabetically.
			return &SQLResponse{
				Results: []map[string]interface{}{
					{"side": "884118"},
					{"side": "553927"},
				},
			}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	resp, err := QueryDistinct(client, "side", &Filters{}, false)
	if err != nil {
		t.Fatalf("QueryDistinct returned error: %v", err)
	}

	if got, want := len(resp.Results), 2; got != want {
		t.Fatalf("len(resp.Results) = %d, want %d", got, want)
	}
	if got := resp.Results[0]["side"]; got != "left" {
		t.Fatalf("first side = %v, want left", got)
	}
	if got := resp.Results[1]["side"]; got != "right" {
		t.Fatalf("second side = %v, want right", got)
	}
	if _, ok := resp.Results[0]["count"]; ok {
		t.Fatal("distinct-without-count result unexpectedly includes count")
	}
}

func TestQueryDistinctSideWithRegionFilterResolvesOptionNamesWithCount(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			if !strings.Contains(sql, "`side`, `region`") {
				t.Fatalf("expected side and region to be selected for Go-side region filtering, got %q", sql)
			}
			return &SQLResponse{
				Results: []map[string]interface{}{
					{"side": "553927", "region": []interface{}{"452098"}},
					{"side": "553927", "region": []interface{}{"452098", "645386"}},
					{"side": "884118", "region": []interface{}{"452098"}},
					{"side": "884118", "region": []interface{}{"645386"}},
				},
			}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	resp, err := QueryDistinct(client, "side", &Filters{Region: "LX"}, true)
	if err != nil {
		t.Fatalf("QueryDistinct returned error: %v", err)
	}

	if got, want := len(resp.Results), 2; got != want {
		t.Fatalf("len(resp.Results) = %d, want %d", got, want)
	}
	if got := resp.Results[0]["side"]; got != "left" {
		t.Fatalf("first side = %v, want left", got)
	}
	if got := resp.Results[0]["count"]; got != 2 {
		t.Fatalf("first count = %v, want 2", got)
	}
	if got := resp.Results[1]["side"]; got != "right" {
		t.Fatalf("second side = %v, want right", got)
	}
	if got := resp.Results[1]["count"]; got != 1 {
		t.Fatalf("second count = %v, want 1", got)
	}
}

func TestQueryDistinctResolvesSideFilterNameForSQL(t *testing.T) {
	var gotSQL string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			gotSQL = sql
			return &SQLResponse{Results: []map[string]interface{}{
				{"cell_type": "ER", "count": float64(2)},
			}}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	_, err := QueryDistinct(client, "cell_type", &Filters{Side: "left"}, true)
	if err != nil {
		t.Fatalf("QueryDistinct returned error: %v", err)
	}

	if !strings.Contains(gotSQL, "`side` = '553927'") {
		t.Fatalf("SQL = %q, want side filter resolved to option ID", gotSQL)
	}
	if strings.Contains(gotSQL, "`side` = 'left'") {
		t.Fatalf("SQL = %q, should not use the user-facing side name in SQL", gotSQL)
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

func TestQueryNeuronInfoResolvesKnownFieldsAndExtras(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			if !strings.Contains(sql, "SELECT *") {
				t.Fatalf("QueryNeuronInfo SQL = %q, want SELECT *", sql)
			}
			return &SQLResponse{
				Results: []map[string]interface{}{
					{
						"_id":             "row-id",
						"root_id":         "111",
						"super_class":     "central",
						"cell_class":      "cx",
						"cell_type":       "EPG/PEG",
						"cell_subtype":    "sub",
						"side":            "left",
						"region":          []interface{}{"452098"},
						"tract":           "tract",
						"nerve":           "nerve",
						"hemilineage":     "hemi",
						"proofread":       "yes",
						"supervoxel_id":   "999",
						"position":        "1, 2, 3",
						"annotation_note": "keep me",
					},
				},
			}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	row, err := QueryNeuronInfo(client, "111")
	if err != nil {
		t.Fatalf("QueryNeuronInfo returned error: %v", err)
	}
	if row == nil {
		t.Fatal("QueryNeuronInfo returned nil row")
	}
	if row.Region != "LX" {
		t.Fatalf("Region = %q, want LX", row.Region)
	}
	if !row.HasPosition() || row.X != 1 || row.Y != 2 || row.Z != 3 {
		t.Fatalf("position = (%v,%v,%v), set=%v; want (1,2,3), true", row.X, row.Y, row.Z, row.HasPosition())
	}
	if row.SupervoxelID != "999" {
		t.Fatalf("SupervoxelID = %q, want 999", row.SupervoxelID)
	}
	if got := row.ExtraFields["annotation_note"]; got != "keep me" {
		t.Fatalf("ExtraFields[annotation_note] = %q, want keep me", got)
	}
	if _, ok := row.ExtraFields["_id"]; ok {
		t.Fatal("ExtraFields unexpectedly includes _id")
	}
}

func TestQueryNeuronInfoPreservesMalformedPosition(t *testing.T) {
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			return &SQLResponse{
				Results: []map[string]interface{}{
					{
						"root_id":  "111",
						"region":   []interface{}{"452098"},
						"position": nil,
					},
				},
			}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	row, err := QueryNeuronInfo(client, "111")
	if err != nil {
		t.Fatalf("QueryNeuronInfo returned error: %v", err)
	}
	if row == nil {
		t.Fatal("QueryNeuronInfo returned nil row")
	}
	if row.HasPosition() {
		t.Fatal("HasPosition() = true, want false")
	}
	if !strings.Contains(row.PositionError, "position value is nil") {
		t.Fatalf("PositionError = %q, want nil-position message", row.PositionError)
	}
}

func TestBuildNeuronInfoRowPrefersNamedColumnsOverMappedKeys(t *testing.T) {
	row := buildNeuronInfoRow(
		map[string]interface{}{
			"key_root":      "mapped-root",
			"root_id":       "named-root",
			"key_region":    []interface{}{"333131"},
			"region":        []interface{}{"452098"},
			"key_position":  "9, 9, 9",
			"position":      "1, 2, 3",
			"key_note":      "mapped note",
			"annotation":    "named note",
			"key_cell_type": "mapped-cell-type",
			"cell_type":     "named-cell-type",
		},
		map[string]string{
			"333131": "CX",
			"452098": "LX",
		},
		map[string]string{
			"key_root":      "root_id",
			"key_region":    "region",
			"key_position":  "position",
			"key_note":      "annotation",
			"key_cell_type": "cell_type",
		},
	)

	if row.RootID != "named-root" {
		t.Fatalf("RootID = %q, want named-root", row.RootID)
	}
	if row.CellType != "named-cell-type" {
		t.Fatalf("CellType = %q, want named-cell-type", row.CellType)
	}
	if row.Region != "LX" {
		t.Fatalf("Region = %q, want LX", row.Region)
	}
	if !row.HasPosition() || row.X != 1 || row.Y != 2 || row.Z != 3 {
		t.Fatalf("position = (%v,%v,%v), set=%v; want (1,2,3), true", row.X, row.Y, row.Z, row.HasPosition())
	}
	if got := row.ExtraFields["annotation"]; got != "named note" {
		t.Fatalf("ExtraFields[annotation] = %q, want named note", got)
	}
	if _, ok := row.ExtraFields["key_note"]; ok {
		t.Fatal("ExtraFields unexpectedly includes raw mapped key")
	}
	if _, ok := row.ExtraFields["key_cell_type"]; ok {
		t.Fatal("ExtraFields unexpectedly includes mapped known field key")
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
		"root_id":       rootID,
		"super_class":   "central",
		"cell_class":    "cx",
		"cell_type":     "EPG/PEG",
		"cell_subtype":  "ER1",
		"cell_instance": "instance-" + rootID,
		"side":          "left",
		"region":        region,
		"tract":         "tract",
		"nerve":         "nerve",
		"hemilineage":   "hemi",
		"proofread":     "true",
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
						{
							Name: "side",
							Data: &ColumnData{
								Options: []SelectOption{
									{ID: "553927", Name: "left"},
									{ID: "884118", Name: "right"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestQueryNeuronsParsesPositionAndToleratesMissingOnes(t *testing.T) {
	positioned := neuronSQLRow("root-pos", []interface{}{"452098"})
	positioned["position"] = "30400, 19771, 2964"
	malformed := neuronSQLRow("root-bad", []interface{}{"452098"})
	malformed["position"] = "not a position"
	absent := neuronSQLRow("root-none", []interface{}{"452098"})

	var sqls []string
	client := &Client{
		executeSQLFunc: func(sql string) (*SQLResponse, error) {
			sqls = append(sqls, sql)
			return &SQLResponse{Results: []map[string]interface{}{positioned, malformed, absent}}, nil
		},
		fetchMetadataFunc: func() (*MetadataResponse, error) {
			return regionMetadata(), nil
		},
	}

	rows, err := QueryNeurons(client, &Filters{})
	if err != nil {
		t.Fatalf("QueryNeurons returned error: %v", err)
	}
	if len(sqls) != 1 {
		t.Fatalf("ExecuteSQL called %d times, want 1", len(sqls))
	}
	if !strings.Contains(sqls[0], "`position`") {
		t.Fatalf("QueryNeurons SQL = %q, want it to select `position`", sqls[0])
	}
	if len(rows) != 3 {
		t.Fatalf("QueryNeurons returned %d rows, want 3", len(rows))
	}

	if !rows[0].PositionSet {
		t.Fatal("root-pos should carry a position")
	}
	if rows[0].X != 30400 || rows[0].Y != 19771 || rows[0].Z != 2964 {
		t.Fatalf("root-pos position = (%g, %g, %g), want (30400, 19771, 2964)", rows[0].X, rows[0].Y, rows[0].Z)
	}
	// A row still counts, and keeps its classification, when its position is
	// unusable; only PositionSet reports the difference.
	for _, row := range rows[1:] {
		if row.PositionSet {
			t.Fatalf("%s should carry no position", row.RootID)
		}
		if row.CellType != "EPG/PEG" {
			t.Fatalf("%s cell_type = %q, want EPG/PEG", row.RootID, row.CellType)
		}
	}
}
