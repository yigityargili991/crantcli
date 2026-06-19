package seatable

// NeuronRow represents a row from the CRANTb_meta table.
type NeuronRow struct {
	RootID         string   `json:"root_id"`
	SuperClass     string   `json:"super_class"`
	CellClass      string   `json:"cell_class"`
	CellType       string   `json:"cell_type"`
	CellSubtype    string   `json:"cell_subtype"`
	Side           string   `json:"side"`
	Region         string   `json:"region"`
	MatchedRegions []string `json:"-"`
	Tract          string   `json:"tract"`
	Nerve          string   `json:"nerve"`
	Hemilineage    string   `json:"hemilineage"`
	Proofread      string   `json:"proofread"`
}

// NeuronPositionRow represents a row with position coordinates from the CRANTb_meta table.
type NeuronPositionRow struct {
	RootID      string
	Region      string
	CellType    string
	Side        string
	X, Y, Z     float64
	PositionSet bool
}

// HasPosition returns true if position data was successfully parsed.
func (n *NeuronPositionRow) HasPosition() bool {
	return n.PositionSet
}

// NeuronCaveCheckRow holds the root ID and supervoxel ID for a CAVE freshness check.
type NeuronCaveCheckRow struct {
	RootID       string
	SupervoxelID string
}

// NeuronInfoRow holds the full row data used by the root-info command.
type NeuronInfoRow struct {
	RootID        string
	SuperClass    string
	CellClass     string
	CellType      string
	CellSubtype   string
	Side          string
	Region        string
	Tract         string
	Nerve         string
	Hemilineage   string
	Proofread     string
	SupervoxelID  string
	PositionRaw   string
	X, Y, Z       float64
	PositionSet   bool
	PositionError string
	ExtraFields   map[string]string
}

// HasPosition returns true if position data was successfully parsed.
func (n *NeuronInfoRow) HasPosition() bool {
	return n != nil && n.PositionSet
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
