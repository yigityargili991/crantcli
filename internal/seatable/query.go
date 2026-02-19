package seatable

import (
	"fmt"
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
	default:
		return fmt.Sprintf("%v", v)
	}
}
