package seatable

// FieldValue returns the string value of a classification field for a neuron
// row. Field names match the CRANTb_meta column names used in filters. For
// "region" it prefers the matched region (set when a query filtered on regions)
// and falls back to the row's resolved region string. Unknown fields return "".
func FieldValue(row NeuronRow, field string) string {
	switch field {
	case "super_class":
		return row.SuperClass
	case "cell_class":
		return row.CellClass
	case "cell_type":
		return row.CellType
	case "cell_subtype":
		return row.CellSubtype
	case "cell_instance":
		return row.CellInstance
	case "side":
		return row.Side
	case "region":
		if len(row.MatchedRegions) > 0 {
			return row.MatchedRegions[0]
		}
		return row.Region
	case "tract":
		return row.Tract
	case "nerve":
		return row.Nerve
	case "hemilineage":
		return row.Hemilineage
	case "proofread":
		return row.Proofread
	default:
		return ""
	}
}
