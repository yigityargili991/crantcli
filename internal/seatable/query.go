package seatable

import (
	"fmt"
	"strconv"
	"strings"

	"crantinject/internal/config"
)

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

// escapeSQL escapes single quotes in a string for safe SQL interpolation.
func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// buildWhere constructs a WHERE clause from the given filters.
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
	add("region", f.Region)
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
	sql := fmt.Sprintf("SELECT `root_id`, `super_class`, `cell_class`, `cell_type`, `cell_subtype`, `side`, `region`, `tract`, `nerve`, `hemilineage`, `proofread` FROM `%s`%s LIMIT 10000",
		config.SeaTableTable, buildWhere(f))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronRow, 0, len(resp.Results))
	for _, r := range resp.Results {
		rows = append(rows, NeuronRow{
			RootID:      toString(r["root_id"]),
			SuperClass:  toString(r["super_class"]),
			CellClass:   toString(r["cell_class"]),
			CellType:    toString(r["cell_type"]),
			CellSubtype: toString(r["cell_subtype"]),
			Side:        toString(r["side"]),
			Region:      toString(r["region"]),
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

	var sql string
	if withCount {
		sql = fmt.Sprintf("SELECT `%s`, COUNT(*) as count FROM `%s`%s GROUP BY `%s` ORDER BY count DESC LIMIT 10000",
			column, config.SeaTableTable, buildWhere(f), column)
	} else {
		sql = fmt.Sprintf("SELECT DISTINCT `%s` FROM `%s`%s ORDER BY `%s` LIMIT 10000",
			column, config.SeaTableTable, buildWhere(f), column)
	}

	return client.ExecuteSQL(sql)
}

// QueryNeuronPosition queries a single neuron's position by root ID.
func QueryNeuronPosition(client *Client, rootID string, regionOpts map[string]string) (*NeuronPositionRow, error) {
	sql := fmt.Sprintf("SELECT `root_id`, `region`, `cell_type`, `position` FROM `%s` WHERE `root_id` = '%s' LIMIT 1",
		config.SeaTableTable, escapeSQL(rootID))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}

	if len(resp.Results) == 0 {
		return nil, nil
	}

	r := resp.Results[0]
	x, y, z := parsePositionValue(r["position"])
	return &NeuronPositionRow{
		RootID:   toString(r["root_id"]),
		Region:   resolveSelectValue(r["region"], regionOpts),
		CellType: toString(r["cell_type"]),
		X:        x,
		Y:        y,
		Z:        z,
	}, nil
}

// QueryNeuronsWithPosition queries all EPG/PEG neurons with their positions.
func QueryNeuronsWithPosition(client *Client, regionOpts map[string]string) ([]NeuronPositionRow, error) {
	f := &Filters{CellType: "EPG/PEG"}
	sql := fmt.Sprintf("SELECT `root_id`, `region`, `cell_type`, `position` FROM `%s`%s LIMIT 10000",
		config.SeaTableTable, buildWhere(f))

	resp, err := client.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}

	rows := make([]NeuronPositionRow, 0, len(resp.Results))
	for _, r := range resp.Results {
		x, y, z := parsePositionValue(r["position"])
		rows = append(rows, NeuronPositionRow{
			RootID:   toString(r["root_id"]),
			Region:   resolveSelectValue(r["region"], regionOpts),
			CellType: toString(r["cell_type"]),
			X:        x,
			Y:        y,
			Z:        z,
		})
	}
	return rows, nil
}

// resolveSelectValue converts a single- or multiple-select value (which may be
// a scalar ID or an array of option IDs) into option name(s).
func resolveSelectValue(v interface{}, opts map[string]string) string {
	if opts == nil || v == nil {
		return toString(v)
	}
	arr, ok := v.([]interface{})
	if !ok {
		// Scalar value — try to resolve as a single-select ID.
		idStr := resolveOptionID(v)
		if name, found := opts[idStr]; found {
			return name
		}
		return toString(v)
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
	return strings.Join(names, ", ")
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
func parsePositionValue(v interface{}) (float64, float64, float64) {
	switch val := v.(type) {
	case string:
		parts := strings.Split(val, ",")
		if len(parts) != 3 {
			return 0, 0, 0
		}
		x, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		y, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		z, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		return x, y, z
	case []interface{}:
		if len(val) != 3 {
			return 0, 0, 0
		}
		return toFloat64(val[0]), toFloat64(val[1]), toFloat64(val[2])
	default:
		return 0, 0, 0
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
