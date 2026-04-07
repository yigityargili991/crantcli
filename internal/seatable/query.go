package seatable

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"crantinject/internal/config"
)

const queryPageSize = 1000
const maxPagedRows = 10000

// Filters holds optional WHERE clause filters for neuron queries.
type Filters struct {
	SuperClass  string
	CellClass   string
	CellType    string
	CellSubtype string
	Side        string
	Region      string
	Tract       string
	Nerve       string
	Hemilineage string
	Proofread   string
}

// HasAny returns true if any filter is set.
func (f *Filters) HasAny() bool {
	return f.SuperClass != "" || f.CellClass != "" || f.CellType != "" ||
		f.CellSubtype != "" || f.Side != "" || f.Region != "" ||
		f.Tract != "" || f.Nerve != "" || f.Hemilineage != "" || f.Proofread != ""
}

// sanitizeIdentifier strips backtick characters from SQL identifiers (table
// and column names) to prevent SQL injection through backtick-quoted names.
func sanitizeIdentifier(name string) string {
	return strings.ReplaceAll(name, "`", "")
}

// escapeSQL escapes single quotes and backslashes in a string for safe SQL
// value interpolation (MySQL-style escaping used by SeaTable).
func escapeSQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", "''")
	return s
}

// buildWhere constructs a WHERE clause from scalar filters.
// Region is filtered in Go because SeaTable returns it as a multi-select array.
func buildWhere(f *Filters) string {
	var conditions []string

	add := func(col, val string) {
		if val != "" {
			conditions = append(conditions, fmt.Sprintf("`%s` = '%s'", col, escapeSQL(val)))
		}
	}

	add("super_class", f.SuperClass)
	add("cell_class", f.CellClass)
	add("cell_type", f.CellType)
	add("cell_subtype", f.CellSubtype)
	add("side", f.Side)
	add("tract", f.Tract)
	add("nerve", f.Nerve)
	add("hemilineage", f.Hemilineage)
	add("proofread", f.Proofread)

	if len(conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conditions, " AND ")
}

// QueryNeurons queries root IDs matching the given filters.
func QueryNeurons(client *Client, f *Filters) ([]NeuronRow, error) {
	regionOpts, regionNameToID, err := loadRegionOptions(client)
	if err != nil {
		return nil, err
	}
	regionFilterID, err := resolveSelectFilterID(f.Region, regionOpts, regionNameToID, "region")
	if err != nil {
		return nil, err
	}

	rowsRaw, err := executePagedSelect(client,
		"`root_id`, `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread`",
		buildWhere(f),
	)
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronRow, 0, len(rowsRaw))
	for _, r := range rowsRaw {
		if regionFilterID != "" && !selectValueContains(r["region"], regionFilterID) {
			continue
		}

		rows = append(rows, NeuronRow{
			RootID:      toString(r["root_id"]),
			SuperClass:  toString(r["super_class"]),
			CellClass:   toString(r["cell_class"]),
			CellType:    toString(r["cell_type"]),
			CellSubtype: toString(r["cell_subtype"]),
			Side:        toString(r["side"]),
			Region:      resolveSelectValue(r["region"], regionOpts),
			Tract:       toString(r["tract"]),
			Nerve:       toString(r["nerve"]),
			Hemilineage: toString(r["hemilineage"]),
			Proofread:   toString(r["proofread"]),
		})
	}
	return rows, nil
}

// QueryDistinct returns distinct values for a given column, optionally with counts.
func QueryDistinct(client *Client, column string, f *Filters, withCount bool) (*SQLResponse, error) {
	validColumns := map[string]bool{
		"super_class": true, "cell_class": true, "cell_type": true,
		"cell_subtype": true, "side": true, "region": true,
		"tract": true, "nerve": true, "hemilineage": true, "proofread": true,
	}
	if !validColumns[column] {
		return nil, fmt.Errorf("invalid column %q; valid columns: super_class, cell_class, cell_type, cell_subtype, side, region, tract, nerve, hemilineage, proofread", column)
	}

	if column == "region" || f.Region != "" {
		return queryDistinctWithRegion(client, column, f, withCount)
	}

	safeCol := sanitizeIdentifier(column)
	safeTable := sanitizeIdentifier(config.SeaTableTable)

	var sql string
	if withCount {
		sql = fmt.Sprintf("SELECT `%s`, COUNT(*) as count FROM `%s`%s GROUP BY `%s` ORDER BY count DESC LIMIT 10000",
			safeCol, safeTable, buildWhere(f), safeCol)
	} else {
		sql = fmt.Sprintf("SELECT DISTINCT `%s` FROM `%s`%s ORDER BY `%s` LIMIT 10000",
			safeCol, safeTable, buildWhere(f), safeCol)
	}

	return client.ExecuteSQL(sql)
}

// QueryNeuronPosition queries a single neuron's position by root ID.
func QueryNeuronPosition(client *Client, rootID string, regionOpts map[string]string) (*NeuronPositionRow, error) {
	sql := fmt.Sprintf("SELECT `root_id`, `region`, `cell_type`, `position` FROM `%s` WHERE `root_id` = '%s' LIMIT 1",
		sanitizeIdentifier(config.SeaTableTable), escapeSQL(rootID))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return nil, nil
	}

	r := resp.Results[0]
	x, y, z, err := parsePositionValue(r["position"])
	row := &NeuronPositionRow{
		RootID:   toString(r["root_id"]),
		Region:   resolveSelectValue(r["region"], regionOpts),
		CellType: toString(r["cell_type"]),
	}
	if err != nil {
		log.Printf("warning: skipping neuron %s: %v", toString(r["root_id"]), err)
		return row, nil
	}
	row.X = x
	row.Y = y
	row.Z = z
	row.PositionSet = true
	return row, nil
}

// QueryNeuronsWithPosition queries all EPG/PEG neurons with their positions.
func QueryNeuronsWithPosition(client *Client, regionOpts map[string]string) ([]NeuronPositionRow, error) {
	f := &Filters{CellType: "EPG/PEG"}
	rowsRaw, err := executePagedSelect(client, "`root_id`, `region`, `cell_type`, `position`", buildWhere(f))
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronPositionRow, 0, len(rowsRaw))
	for _, r := range rowsRaw {
		x, y, z, err := parsePositionValue(r["position"])
		if err != nil {
			log.Printf("warning: skipping neuron %s: %v", toString(r["root_id"]), err)
			continue
		}
		rows = append(rows, NeuronPositionRow{
			RootID:      toString(r["root_id"]),
			Region:      resolveSelectValue(r["region"], regionOpts),
			CellType:    toString(r["cell_type"]),
			X:           x,
			Y:           y,
			Z:           z,
			PositionSet: true,
		})
	}
	return rows, nil
}

// QueryNeuronSupervoxel looks up the supervoxel_id for a single root ID.
func QueryNeuronSupervoxel(client *Client, rootID string) (*NeuronCaveCheckRow, error) {
	sql := fmt.Sprintf("SELECT `root_id`, `%s` FROM `%s` WHERE `root_id` = '%s' LIMIT 1",
		sanitizeIdentifier(config.SupervoxelIDColumn),
		sanitizeIdentifier(config.SeaTableTable),
		escapeSQL(rootID))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return nil, nil
	}

	r := resp.Results[0]
	return &NeuronCaveCheckRow{
		RootID:       toString(r["root_id"]),
		SupervoxelID: toString(r[config.SupervoxelIDColumn]),
	}, nil
}

// QueryNeuronsForCaveCheck returns root_id and supervoxel_id for neurons matching filters.
func QueryNeuronsForCaveCheck(client *Client, f *Filters) ([]NeuronCaveCheckRow, error) {
	columns := fmt.Sprintf("`root_id`, `%s`", sanitizeIdentifier(config.SupervoxelIDColumn))

	if f.Region != "" {
		columns += ", `region`"
	}

	regionFilterID := ""
	if f.Region != "" {
		regionOpts, regionNameToID, err := loadRegionOptions(client)
		if err != nil {
			return nil, err
		}
		regionFilterID, err = resolveSelectFilterID(f.Region, regionOpts, regionNameToID, "region")
		if err != nil {
			return nil, err
		}
	}

	rowsRaw, err := executePagedSelect(client, columns, buildWhere(f))
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronCaveCheckRow, 0, len(rowsRaw))
	for _, r := range rowsRaw {
		if regionFilterID != "" && !selectValueContains(r["region"], regionFilterID) {
			continue
		}
		rows = append(rows, NeuronCaveCheckRow{
			RootID:       toString(r["root_id"]),
			SupervoxelID: toString(r[config.SupervoxelIDColumn]),
		})
	}
	return rows, nil
}

func queryDistinctWithRegion(client *Client, column string, f *Filters, withCount bool) (*SQLResponse, error) {
	regionOpts, regionNameToID, err := loadRegionOptions(client)
	if err != nil {
		return nil, err
	}
	regionFilterID, err := resolveSelectFilterID(f.Region, regionOpts, regionNameToID, "region")
	if err != nil {
		return nil, err
	}

	selectColumns := fmt.Sprintf("`%s`, `region`", column)
	if column == "region" {
		selectColumns = "`region`"
	}

	rowsRaw, err := executePagedSelect(client, selectColumns, buildWhere(f))
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, row := range rowsRaw {
		if regionFilterID != "" && !selectValueContains(row["region"], regionFilterID) {
			continue
		}

		if column == "region" {
			names := uniqueStrings(resolveSelectValues(row["region"], regionOpts))
			for _, name := range names {
				if name == "" {
					continue
				}
				counts[name]++
			}
			continue
		}

		value := toString(row[column])
		if value == "" {
			continue
		}
		counts[value]++
	}

	return buildDistinctResponse(column, counts, withCount), nil
}

func buildDistinctResponse(column string, counts map[string]int, withCount bool) *SQLResponse {
	type entry struct {
		value string
		count int
	}

	items := make([]entry, 0, len(counts))
	for value, count := range counts {
		items = append(items, entry{value: value, count: count})
	}

	if withCount {
		sort.Slice(items, func(i, j int) bool {
			if items[i].count != items[j].count {
				return items[i].count > items[j].count
			}
			return items[i].value < items[j].value
		})
	} else {
		sort.Slice(items, func(i, j int) bool {
			return items[i].value < items[j].value
		})
	}

	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		row := map[string]interface{}{column: item.value}
		if withCount {
			row["count"] = item.count
		}
		results = append(results, row)
	}

	return &SQLResponse{Results: results}
}

func executePagedSelect(client *Client, columns string, where string) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}

	for offset := 0; ; offset += queryPageSize {
		sql := fmt.Sprintf("SELECT %s FROM `%s`%s ORDER BY _id LIMIT %d OFFSET %d",
			columns, sanitizeIdentifier(config.SeaTableTable), where, queryPageSize, offset)
		resp, err := client.ExecuteSQL(sql)
		if err != nil {
			return nil, err
		}

		if len(rows)+len(resp.Results) > maxPagedRows {
			return nil, fmt.Errorf("query exceeded %d row safety limit; add filters to narrow results", maxPagedRows)
		}
		rows = append(rows, resp.Results...)
		if len(resp.Results) < queryPageSize {
			break
		}
	}

	return rows, nil
}

func loadRegionOptions(client *Client) (map[string]string, map[string]string, error) {
	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, nil, fmt.Errorf("fetching column metadata: %w", err)
	}
	return SelectOptionMap(meta, "region"), SelectOptionNameMap(meta, "region"), nil
}

func resolveSelectFilterID(input string, idToName map[string]string, nameToID map[string]string, field string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}
	if _, ok := idToName[trimmed]; ok {
		return trimmed, nil
	}

	lower := strings.ToLower(trimmed)
	if id, ok := nameToID[lower]; ok {
		return id, nil
	}

	return "", fmt.Errorf("unknown %s %q", field, input)
}

func selectValueContains(v interface{}, targetID string) bool {
	if targetID == "" || v == nil {
		return false
	}

	arr, ok := v.([]interface{})
	if !ok {
		return resolveOptionID(v) == targetID
	}

	for _, elem := range arr {
		if resolveOptionID(elem) == targetID {
			return true
		}
	}
	return false
}

// resolveSelectValue converts a single- or multiple-select value (which may be
// a scalar ID or an array of option IDs) into option name(s).
func resolveSelectValue(v interface{}, opts map[string]string) string {
	return strings.Join(resolveSelectValues(v, opts), ", ")
}

func resolveSelectValues(v interface{}, opts map[string]string) []string {
	if v == nil {
		return nil
	}
	if opts == nil {
		return []string{toString(v)}
	}

	arr, ok := v.([]interface{})
	if !ok {
		idStr := resolveOptionID(v)
		if name, found := opts[idStr]; found {
			return []string{name}
		}
		return []string{toString(v)}
	}

	names := make([]string, 0, len(arr))
	for _, elem := range arr {
		idStr := resolveOptionID(elem)
		if name, found := opts[idStr]; found {
			names = append(names, name)
		} else {
			names = append(names, idStr)
		}
	}
	return names
}

// resolveOptionID converts a value to a string suitable for option map lookup.
func resolveOptionID(v interface{}) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%v", v)
}

// parsePositionValue extracts x, y, z from a position value, which may be
// a comma-separated string ("30400, 19771, 2964") or a JSON array [30400, 19771, 2964].
func parsePositionValue(v interface{}) (float64, float64, float64, error) {
	switch val := v.(type) {
	case string:
		parts := strings.Split(val, ",")
		if len(parts) != 3 {
			return 0, 0, 0, fmt.Errorf("position string has %d components, want 3: %q", len(parts), val)
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parsing x component %q: %w", strings.TrimSpace(parts[0]), err)
		}
		y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parsing y component %q: %w", strings.TrimSpace(parts[1]), err)
		}
		z, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parsing z component %q: %w", strings.TrimSpace(parts[2]), err)
		}
		return x, y, z, nil
	case []interface{}:
		if len(val) != 3 {
			return 0, 0, 0, fmt.Errorf("position array has %d elements, want 3", len(val))
		}
		x, err := parsePositionComponent(val[0])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parsing x component: %w", err)
		}
		y, err := parsePositionComponent(val[1])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parsing y component: %w", err)
		}
		z, err := parsePositionComponent(val[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("parsing z component: %w", err)
		}
		return x, y, z, nil
	default:
		if v == nil {
			return 0, 0, 0, fmt.Errorf("position value is nil")
		}
		return 0, 0, 0, fmt.Errorf("unrecognized position type %T", v)
	}
}

func parsePositionComponent(v interface{}) (float64, error) {
	switch val := v.(type) {
	case nil:
		return 0, fmt.Errorf("component is nil")
	case float64:
		return val, nil
	case string:
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			return 0, fmt.Errorf("component is empty")
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid numeric value %q: %w", trimmed, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("unsupported component type %T", v)
	}
}

func toFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, elem := range val {
			s := toString(elem)
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
