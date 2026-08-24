package seatable

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"crantcli/internal/config"
)

const queryPageSize = 1000
const maxPagedRows = 100000

// Filters holds optional WHERE clause filters for neuron queries.
type Filters struct {
	SuperClass  string
	CellClass   string
	CellType    string
	CellSubtype string
	RootIDs     []string
	Side        string
	Region      string
	Regions     []string
	Tract       string
	Nerve       string
	Hemilineage string
	Proofread   string
}

// HasAny returns true if any filter is set.
func (f *Filters) HasAny() bool {
	if f == nil {
		return false
	}
	return f.SuperClass != "" || f.CellClass != "" || f.CellType != "" ||
		f.CellSubtype != "" || len(f.RootIDs) > 0 || f.Side != "" || len(f.regionValues()) > 0 ||
		f.Tract != "" || f.Nerve != "" || f.Hemilineage != "" || f.Proofread != ""
}

func (f *Filters) regionValues() []string {
	if f == nil {
		return nil
	}

	values := make([]string, 0, len(f.Regions)+1)
	if strings.TrimSpace(f.Region) != "" {
		values = append(values, f.Region)
	}
	values = append(values, f.Regions...)
	return compactUniqueStrings(values)
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
	if f == nil {
		return ""
	}

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

	if rootIDs := compactUniqueStrings(f.RootIDs); len(rootIDs) > 0 {
		quoted := make([]string, len(rootIDs))
		for i, rootID := range rootIDs {
			quoted[i] = "'" + escapeSQL(rootID) + "'"
		}
		conditions = append(conditions, "`root_id` IN ("+strings.Join(quoted, ", ")+")")
	}

	if len(conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conditions, " AND ")
}

func filtersWithResolvedSideFromClient(client *Client, f *Filters) (*Filters, error) {
	if f == nil || strings.TrimSpace(f.Side) == "" {
		return f, nil
	}
	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("fetching column metadata: %w", err)
	}
	return filtersWithResolvedSide(f, meta)
}

func filtersWithResolvedSide(f *Filters, meta *MetadataResponse) (*Filters, error) {
	if f == nil || strings.TrimSpace(f.Side) == "" {
		return f, nil
	}
	sideID, err := resolveSelectFilterID(f.Side, SelectOptionMap(meta, "side"), SelectOptionNameMap(meta, "side"), "side")
	if err != nil {
		return nil, err
	}
	resolved := *f
	resolved.Side = sideID
	return &resolved, nil
}

// QueryNeurons queries root IDs matching the given filters.
func QueryNeurons(client *Client, f *Filters) ([]NeuronRow, error) {
	regionOpts, regionNameToID, err := loadRegionOptions(client)
	if err != nil {
		return nil, err
	}
	regionFilterIDs, err := resolveSelectFilterIDs(f.regionValues(), regionOpts, regionNameToID, "region")
	if err != nil {
		return nil, err
	}

	// side is a single-select column; resolve its option IDs to names. Metadata
	// is cached, so this does not incur an extra request.
	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("fetching column metadata: %w", err)
	}
	sideOpts := SelectOptionMap(meta, "side")
	sqlFilters, err := filtersWithResolvedSide(f, meta)
	if err != nil {
		return nil, err
	}

	rowsRaw, err := executePagedSelect(client,
		"`root_id`, `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `cell_instance`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread`, `position`",
		buildWhere(sqlFilters),
	)
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronRow, 0, len(rowsRaw))
	for _, r := range rowsRaw {
		if len(regionFilterIDs) > 0 && !selectValueContainsAny(r["region"], regionFilterIDs) {
			continue
		}

		regionValues := resolveSelectValues(r["region"], regionOpts)
		row := NeuronRow{
			RootID:         toString(r["root_id"]),
			SuperClass:     toString(r["super_class"]),
			CellClass:      toString(r["cell_class"]),
			CellType:       toString(r["cell_type"]),
			CellSubtype:    toString(r["cell_subtype"]),
			CellInstance:   toString(r["cell_instance"]),
			Side:           resolveSelectValue(r["side"], sideOpts),
			Region:         strings.Join(regionValues, ", "),
			MatchedRegions: matchedSelectValues(r["region"], regionFilterIDs, regionOpts),
			Tract:          toString(r["tract"]),
			Nerve:          toString(r["nerve"]),
			Hemilineage:    toString(r["hemilineage"]),
			Proofread:      toString(r["proofread"]),
		}
		// A missing or malformed position is common and never fatal here: the
		// row still carries its classification, only without coordinates.
		if x, y, z, err := parsePositionValue(r["position"]); err == nil {
			row.X, row.Y, row.Z = x, y, z
			row.PositionSet = true
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// QueryNeuronsByRootIDs returns the metadata rows matching the given root IDs.
func QueryNeuronsByRootIDs(client *Client, rootIDs []string) ([]NeuronRow, error) {
	if len(compactUniqueStrings(rootIDs)) == 0 {
		return nil, nil
	}
	return QueryNeurons(client, &Filters{RootIDs: rootIDs})
}

// QueryDistinct returns distinct values for a given column, optionally with counts.
func QueryDistinct(client *Client, column string, f *Filters, withCount bool) (*SQLResponse, error) {
	validColumns := map[string]bool{
		"super_class": true, "cell_class": true, "cell_type": true,
		"cell_subtype": true, "cell_instance": true, "side": true, "region": true,
		"tract": true, "nerve": true, "hemilineage": true, "proofread": true,
	}
	if !validColumns[column] {
		return nil, fmt.Errorf("invalid column %q; valid columns: super_class, cell_class, cell_type, cell_subtype, cell_instance, side, region, tract, nerve, hemilineage, proofread", column)
	}

	if column == "region" || len(f.regionValues()) > 0 {
		return queryDistinctWithRegion(client, column, f, withCount)
	}

	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("fetching column metadata: %w", err)
	}
	sqlFilters, err := filtersWithResolvedSide(f, meta)
	if err != nil {
		return nil, err
	}

	var sql string
	if withCount {
		sql = fmt.Sprintf("SELECT `%s`, COUNT(*) as count FROM `%s`%s GROUP BY `%s` ORDER BY count DESC LIMIT 10000",
			column, config.SeaTableTable, buildWhere(sqlFilters), column)
	} else {
		sql = fmt.Sprintf("SELECT DISTINCT `%s` FROM `%s`%s ORDER BY `%s` LIMIT 10000",
			column, config.SeaTableTable, buildWhere(sqlFilters), column)
	}

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}

	return resolveDistinctSelectValues(meta, column, resp, withCount), nil
}

// resolveDistinctSelectValues maps single-select option IDs in a distinct/count
// response to their option names (e.g. side: "553927" -> "left"), aggregating
// counts under the resolved name. Columns without select options (plain text
// columns) are returned unchanged, so this is a no-op for them.
func resolveDistinctSelectValues(meta *MetadataResponse, column string, resp *SQLResponse, withCount bool) *SQLResponse {
	opts := SelectOptionMap(meta, column)
	if len(opts) == 0 {
		return resp
	}

	counts := make(map[string]int)
	for _, row := range resp.Results {
		name := resolveSelectValue(row[column], opts)
		if name == "" {
			continue
		}
		if withCount {
			counts[name] += toInt(row["count"])
		} else {
			counts[name]++
		}
	}

	return buildDistinctResponse(column, counts, withCount)
}

// QueryNeuronPosition queries a single neuron's position by root ID.
func QueryNeuronPosition(client *Client, rootID string, regionOpts map[string]string) (*NeuronPositionRow, error) {
	sql := fmt.Sprintf("SELECT `root_id`, `region`, `cell_type`, `side`, `position` FROM `%s` WHERE `root_id` = '%s' LIMIT 1",
		config.SeaTableTable, escapeSQL(rootID))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return nil, nil
	}

	row, err := buildNeuronPositionRow(resp.Results[0], regionOpts)
	if err != nil {
		log.Printf("warning: skipping neuron %s: %v", row.RootID, err)
		return &row, nil
	}
	return &row, nil
}

// QueryNeuronsWithPosition queries all EPG/PEG neurons with their positions.
func QueryNeuronsWithPosition(client *Client, regionOpts map[string]string) ([]NeuronPositionRow, error) {
	f := &Filters{CellType: "EPG/PEG"}
	return queryNeuronPositions(client, f, regionOpts, false)
}

// QueryNeuronPositions queries neurons matching the filters and preserves rows
// whose position field is missing or malformed.
func QueryNeuronPositions(client *Client, f *Filters, regionOpts map[string]string) ([]NeuronPositionRow, error) {
	return queryNeuronPositions(client, f, regionOpts, true)
}

func queryNeuronPositions(client *Client, f *Filters, regionOpts map[string]string, preserveInvalid bool) ([]NeuronPositionRow, error) {
	if f == nil {
		f = &Filters{}
	}

	regionFilterIDs := []string(nil)
	if len(f.regionValues()) > 0 {
		var err error
		regionFilterIDs, err = resolveSelectFilterIDs(f.regionValues(), regionOpts, selectOptionNameMap(regionOpts), "region")
		if err != nil {
			return nil, err
		}
	}

	sqlFilters, err := filtersWithResolvedSideFromClient(client, f)
	if err != nil {
		return nil, err
	}

	rowsRaw, err := executePagedSelect(client, "`root_id`, `region`, `cell_type`, `side`, `position`", buildWhere(sqlFilters))
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronPositionRow, 0, len(rowsRaw))
	for _, r := range rowsRaw {
		if len(regionFilterIDs) > 0 && !selectValueContainsAny(r["region"], regionFilterIDs) {
			continue
		}

		row, err := buildNeuronPositionRow(r, regionOpts)
		// With preserveInvalid, return malformed target rows rather than
		// logging/skipping them; side-check inspects PositionSet/HasPosition.
		if err != nil && !preserveInvalid {
			log.Printf("warning: skipping neuron %s: %v", row.RootID, err)
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func buildNeuronPositionRow(r map[string]interface{}, regionOpts map[string]string) (NeuronPositionRow, error) {
	row := NeuronPositionRow{
		RootID:   toString(r["root_id"]),
		Region:   resolveSelectValue(r["region"], regionOpts),
		CellType: toString(r["cell_type"]),
		Side:     toString(r["side"]),
	}

	x, y, z, err := parsePositionValue(r["position"])
	if err != nil {
		return row, err
	}
	row.X = x
	row.Y = y
	row.Z = z
	row.PositionSet = true
	return row, nil
}

func selectOptionNameMap(idToName map[string]string) map[string]string {
	m := make(map[string]string, len(idToName))
	for id, name := range idToName {
		m[strings.ToLower(strings.TrimSpace(name))] = id
	}
	return m
}

// QueryNeuronSupervoxel looks up the supervoxel_id for a single root ID.
func QueryNeuronSupervoxel(client *Client, rootID string) (*NeuronCaveCheckRow, error) {
	sql := fmt.Sprintf("SELECT `root_id`, `%s` FROM `%s` WHERE `root_id` = '%s' LIMIT 1",
		config.SupervoxelIDColumn,
		config.SeaTableTable,
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

// QueryNeuronInfo looks up a single neuron row by root ID and resolves fields
// needed for root-info, preserving unknown columns as display strings.
func QueryNeuronInfo(client *Client, rootID string) (*NeuronInfoRow, error) {
	sql := fmt.Sprintf("SELECT * FROM `%s` WHERE `root_id` = '%s' LIMIT 1",
		config.SeaTableTable,
		escapeSQL(rootID))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0] == nil {
		return nil, nil
	}

	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("fetching column metadata: %w", err)
	}

	row := buildNeuronInfoRow(resp.Results[0], SelectOptionMap(meta, "region"), tableColumnKeyToName(meta))
	return &row, nil
}

func buildNeuronInfoRow(r map[string]interface{}, regionOpts map[string]string, keyToName map[string]string) NeuronInfoRow {
	values := canonicalColumnValues(r, keyToName)
	get := func(name string) (interface{}, bool) {
		value, ok := values[name]
		return value, ok
	}

	row := NeuronInfoRow{
		RootID:       toStringValue(get("root_id")),
		SuperClass:   toStringValue(get("super_class")),
		CellClass:    toStringValue(get("cell_class")),
		CellType:     toStringValue(get("cell_type")),
		CellSubtype:  toStringValue(get("cell_subtype")),
		Side:         toStringValue(get("side")),
		Region:       resolveSelectValueFromLookup(get, "region", regionOpts),
		Tract:        toStringValue(get("tract")),
		Nerve:        toStringValue(get("nerve")),
		Hemilineage:  toStringValue(get("hemilineage")),
		Proofread:    toStringValue(get("proofread")),
		SupervoxelID: toStringValue(get(config.SupervoxelIDColumn)),
		ExtraFields:  make(map[string]string),
	}

	if position, ok := get("position"); ok {
		row.PositionRaw = toString(position)
		x, y, z, err := parsePositionValue(position)
		if err != nil {
			row.PositionError = err.Error()
		} else {
			row.X = x
			row.Y = y
			row.Z = z
			row.PositionSet = true
		}
	} else {
		row.PositionError = "position missing"
	}

	for name, value := range values {
		if isRootInfoKnownField(name) {
			continue
		}
		display := toString(value)
		if display == "" {
			continue
		}
		row.ExtraFields[name] = display
	}

	return row
}

func canonicalColumnValues(r map[string]interface{}, keyToName map[string]string) map[string]interface{} {
	// SeaTable rows may contain both raw column keys and column names; map keys
	// first, then let direct named columns intentionally override duplicates.
	values := make(map[string]interface{}, len(r))
	for key, value := range r {
		name := key
		if mapped, ok := keyToName[key]; ok {
			name = mapped
		}
		values[name] = value
	}
	for key, value := range r {
		if _, isMappedKey := keyToName[key]; isMappedKey {
			continue
		}
		values[key] = value
	}
	return values
}

func toStringValue(v interface{}, ok bool) string {
	if !ok {
		return ""
	}
	return toString(v)
}

func resolveSelectValueFromLookup(get func(string) (interface{}, bool), name string, opts map[string]string) string {
	value, ok := get(name)
	if !ok {
		return ""
	}
	return resolveSelectValue(value, opts)
}

func tableColumnKeyToName(meta *MetadataResponse) map[string]string {
	result := make(map[string]string)
	if meta == nil {
		return result
	}
	for _, table := range meta.Metadata.Tables {
		if table.Name != config.SeaTableTable {
			continue
		}
		for _, col := range table.Columns {
			if col.Key == "" || col.Name == "" {
				continue
			}
			result[col.Key] = col.Name
		}
	}
	return result
}

func isRootInfoKnownField(name string) bool {
	switch name {
	case "_id",
		"root_id",
		"super_class",
		"cell_class",
		"cell_type",
		"cell_subtype",
		"side",
		"region",
		"tract",
		"nerve",
		"hemilineage",
		"proofread",
		"position",
		config.SupervoxelIDColumn:
		return true
	default:
		return false
	}
}

// QueryNeuronsForCaveCheck returns root_id and supervoxel_id for neurons matching filters.
func QueryNeuronsForCaveCheck(client *Client, f *Filters) ([]NeuronCaveCheckRow, error) {
	columns := fmt.Sprintf("`root_id`, `%s`", config.SupervoxelIDColumn)

	regionFilterIDs := []string(nil)
	if len(f.regionValues()) > 0 {
		columns += ", `region`"
		regionOpts, regionNameToID, err := loadRegionOptions(client)
		if err != nil {
			return nil, err
		}
		regionFilterIDs, err = resolveSelectFilterIDs(f.regionValues(), regionOpts, regionNameToID, "region")
		if err != nil {
			return nil, err
		}
	}

	sqlFilters, err := filtersWithResolvedSideFromClient(client, f)
	if err != nil {
		return nil, err
	}

	rowsRaw, err := executePagedSelect(client, columns, buildWhere(sqlFilters))
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronCaveCheckRow, 0, len(rowsRaw))
	for _, r := range rowsRaw {
		if len(regionFilterIDs) > 0 && !selectValueContainsAny(r["region"], regionFilterIDs) {
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
	meta, err := client.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("fetching column metadata: %w", err)
	}
	regionOpts := SelectOptionMap(meta, "region")
	regionNameToID := SelectOptionNameMap(meta, "region")
	regionFilterIDs, err := resolveSelectFilterIDs(f.regionValues(), regionOpts, regionNameToID, "region")
	if err != nil {
		return nil, err
	}
	columnOpts := SelectOptionMap(meta, column)
	sqlFilters, err := filtersWithResolvedSide(f, meta)
	if err != nil {
		return nil, err
	}

	selectColumns := fmt.Sprintf("`%s`, `region`", column)
	if column == "region" {
		selectColumns = "`region`"
	}

	rowsRaw, err := executePagedSelect(client, selectColumns, buildWhere(sqlFilters))
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, row := range rowsRaw {
		if len(regionFilterIDs) > 0 && !selectValueContainsAny(row["region"], regionFilterIDs) {
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

		value := resolveSelectValue(row[column], columnOpts)
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
			columns, config.SeaTableTable, where, queryPageSize, offset)
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

func resolveSelectFilterIDs(inputs []string, idToName map[string]string, nameToID map[string]string, field string) ([]string, error) {
	ids := make([]string, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		id, err := resolveSelectFilterID(input, idToName, nameToID, field)
		if err != nil {
			return nil, err
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
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

func selectValueContainsAny(v interface{}, targetIDs []string) bool {
	for _, targetID := range targetIDs {
		if selectValueContains(v, targetID) {
			return true
		}
	}
	return false
}

func matchedSelectValues(v interface{}, targetIDs []string, opts map[string]string) []string {
	matches := make([]string, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		if !selectValueContains(v, targetID) {
			continue
		}
		if name, found := opts[targetID]; found {
			matches = append(matches, name)
			continue
		}
		matches = append(matches, targetID)
	}
	return matches
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

// toInt coerces a SQL aggregate value (COUNT(*) decodes from JSON as float64)
// to an int, returning 0 for unexpected types.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
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

func compactUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
