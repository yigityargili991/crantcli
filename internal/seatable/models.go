package seatable

// NeuronRow represents a row from the CRANTb_meta table.
type NeuronRow struct {
	RootID      string `json:"root_id"`
	SuperClass  string `json:"super_class"`
	CellClass   string `json:"cell_class"`
	CellType    string `json:"cell_type"`
	CellSubtype string `json:"cell_subtype"`
	Side        string `json:"side"`
	Region      string `json:"region"`
	Tract       string `json:"tract"`
	Nerve       string `json:"nerve"`
	Hemilineage string `json:"hemilineage"`
	Proofread   string `json:"proofread"`
}

// NeuronPositionRow represents a row with position coordinates from the CRANTb_meta table.
type NeuronPositionRow struct {
	RootID   string
	Region   string
	CellType string
	X, Y, Z  float64
}

// HasPosition returns true if any coordinate is non-zero.
func (n *NeuronPositionRow) HasPosition() bool {
	return n.X != 0 || n.Y != 0 || n.Z != 0
}

// AuthResponse is returned by the SeaTable app-access-token endpoint.
type AuthResponse struct {
	AccessToken string `json:"access_token"`
	DTableUUID  string `json:"dtable_uuid"`
}

// SQLResponse is the response from the SQL query endpoint.
type SQLResponse struct {
	Metadata []ColumnMeta             `json:"metadata"`
	Results  []map[string]interface{} `json:"results"`
}

// ColumnMeta describes a column in the SQL response.
type ColumnMeta struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type"`
}
