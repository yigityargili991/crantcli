package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"crantcli/internal/cave"
	"crantcli/internal/seatable"

	"github.com/spf13/cobra"
)

type rootInfoDataSource interface {
	QueryNeuronInfo(rootID string) (*seatable.NeuronInfoRow, error)
	RegionOptions() (map[string]string, error)
	QueryEPGPEGPositions(regionOpts map[string]string) ([]seatable.NeuronPositionRow, error)
}

type rootInfoCaveClient interface {
	GetRootID(supervoxelID uint64) (uint64, error)
	GetRootChangeLog(rootID uint64, filtered bool) ([]cave.ChangeLogRow, error)
}

type seatableRootInfoSource struct {
	client *seatable.Client
}

func (s seatableRootInfoSource) QueryNeuronInfo(rootID string) (*seatable.NeuronInfoRow, error) {
	return seatable.QueryNeuronInfo(s.client, rootID)
}

func (s seatableRootInfoSource) RegionOptions() (map[string]string, error) {
	meta, err := s.client.FetchMetadata()
	if err != nil {
		return nil, fmt.Errorf("fetching column metadata: %w", err)
	}
	return seatable.SelectOptionMap(meta, "region"), nil
}

func (s seatableRootInfoSource) QueryEPGPEGPositions(regionOpts map[string]string) ([]seatable.NeuronPositionRow, error) {
	return seatable.QueryNeuronsWithPosition(s.client, regionOpts)
}

type rootInfoOptions struct {
	JSON         bool
	HistoryLimit int
	Filtered     bool
}

type rootInfoResult struct {
	RootID         string                 `json:"root_id"`
	Classification rootInfoClassification `json:"classification"`
	Position       *rootInfoPosition      `json:"position"`
	PositionError  string                 `json:"position_error,omitempty"`
	CAVE           rootInfoCAVEStatus     `json:"cave"`
	NearestColumn  *rootInfoNearestColumn `json:"nearest_column"`
	NearestError   string                 `json:"nearest_column_error,omitempty"`
	History        rootInfoHistory        `json:"history"`
	ExtraFields    map[string]string      `json:"extra_fields,omitempty"`
}

type rootInfoClassification struct {
	SuperClass  string `json:"super_class,omitempty"`
	CellClass   string `json:"cell_class,omitempty"`
	CellType    string `json:"cell_type,omitempty"`
	CellSubtype string `json:"cell_subtype,omitempty"`
	Side        string `json:"side,omitempty"`
	Region      string `json:"region,omitempty"`
	Tract       string `json:"tract,omitempty"`
	Nerve       string `json:"nerve,omitempty"`
	Hemilineage string `json:"hemilineage,omitempty"`
	Proofread   string `json:"proofread,omitempty"`
}

type rootInfoPosition struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Z   float64 `json:"z"`
	Raw string  `json:"raw,omitempty"`
}

type rootInfoCAVEStatus struct {
	SupervoxelID  string `json:"supervoxel_id,omitempty"`
	CurrentRootID string `json:"current_root_id,omitempty"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

type rootInfoNearestColumn struct {
	Region       string  `json:"region,omitempty"`
	RootID       string  `json:"root_id,omitempty"`
	Side         string  `json:"side,omitempty"`
	Distance     float64 `json:"distance"`
	SideRelation string  `json:"side_relation,omitempty"`
}

type rootInfoHistory struct {
	Filtered bool               `json:"filtered"`
	Total    int                `json:"total"`
	Merges   int                `json:"merges"`
	Splits   int                `json:"splits"`
	Error    string             `json:"error,omitempty"`
	Latest   *caveHistoryEntry  `json:"latest,omitempty"`
	Entries  []caveHistoryEntry `json:"entries"`
}

var rootInfoCmd = &cobra.Command{
	Use:         "root-info <root_id>",
	Short:       "Show CRANT, CAVE, and nearest-column info for a root ID",
	Long:        `Show CRANT metadata, CAVE current-root status, edit history, and nearest EPG/PEG column context for a root ID.`,
	Annotations: map[string]string{"requiresToken": "true"},
	Args:        cobra.ExactArgs(1),
}

func init() {
	var (
		asJSON       bool
		historyLimit int
		unfiltered   bool
	)

	rootInfoCmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON output")
	rootInfoCmd.Flags().IntVar(&historyLimit, "history-limit", 5, "Number of recent CAVE history entries to print")
	rootInfoCmd.Flags().BoolVar(&unfiltered, "unfiltered", false, "Include unfiltered split/merge history")
	rootInfoCmd.ValidArgsFunction = noFileCompletion

	rootInfoCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if historyLimit < 0 {
			return fmt.Errorf("--history-limit must be >= 0")
		}

		stClient, err := seatable.NewClient()
		if err != nil {
			return err
		}
		caveClient, err := cave.NewClient()
		if err != nil {
			return err
		}

		return runRootInfo(cmd.OutOrStdout(), seatableRootInfoSource{client: stClient}, caveClient, args[0], rootInfoOptions{
			JSON:         asJSON,
			HistoryLimit: historyLimit,
			Filtered:     !unfiltered,
		})
	}

	rootCmd.AddCommand(rootInfoCmd)
}

func runRootInfo(out io.Writer, data rootInfoDataSource, caveClient rootInfoCaveClient, rawRootID string, opts rootInfoOptions) error {
	result, err := fetchRootInfo(data, caveClient, rawRootID, opts)
	if err != nil {
		return err
	}
	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return writeRootInfoText(out, result)
}

func fetchRootInfo(data rootInfoDataSource, caveClient rootInfoCaveClient, rawRootID string, opts rootInfoOptions) (*rootInfoResult, error) {
	rootID, err := parseRootInfoRootID(rawRootID)
	if err != nil {
		return nil, err
	}
	rootIDString := strconv.FormatUint(rootID, 10)

	row, err := data.QueryNeuronInfo(rootIDString)
	if err != nil {
		return nil, fmt.Errorf("querying root_id %s: %w", rootIDString, err)
	}
	if row == nil {
		return nil, fmt.Errorf("no neuron found with root_id %q", rootIDString)
	}
	if strings.TrimSpace(row.RootID) == "" {
		row.RootID = rootIDString
	}

	historyRows, err := caveClient.GetRootChangeLog(rootID, opts.Filtered)
	history := summarizeRootInfoHistory(historyRows, opts.Filtered, opts.HistoryLimit)
	if err != nil {
		if !errors.Is(err, cave.ErrChangeLogTimeout) {
			return nil, fmt.Errorf("fetching history for root_id %s: %w", rootIDString, err)
		}
		history.Error = err.Error()
	}

	result := &rootInfoResult{
		RootID: row.RootID,
		Classification: rootInfoClassification{
			SuperClass:  row.SuperClass,
			CellClass:   row.CellClass,
			CellType:    row.CellType,
			CellSubtype: row.CellSubtype,
			Side:        row.Side,
			Region:      row.Region,
			Tract:       row.Tract,
			Nerve:       row.Nerve,
			Hemilineage: row.Hemilineage,
			Proofread:   row.Proofread,
		},
		CAVE:        buildRootInfoCAVEStatus(caveClient, row),
		History:     history,
		ExtraFields: row.ExtraFields,
	}

	if row.HasPosition() {
		result.Position = &rootInfoPosition{X: row.X, Y: row.Y, Z: row.Z, Raw: row.PositionRaw}
		nearest, nearestErr, err := fetchNearestColumn(data, row)
		if err != nil {
			return nil, err
		}
		result.NearestColumn = nearest
		result.NearestError = nearestErr
	} else {
		result.PositionError = row.PositionError
		if result.PositionError == "" {
			result.PositionError = "position unavailable"
		}
		result.NearestError = "position unavailable"
	}

	return result, nil
}

func parseRootInfoRootID(raw string) (uint64, error) {
	rootID, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid root_id %q: %w", raw, err)
	}
	return rootID, nil
}

func buildRootInfoCAVEStatus(client rootInfoCaveClient, row *seatable.NeuronInfoRow) rootInfoCAVEStatus {
	status := rootInfoCAVEStatus{
		SupervoxelID: row.SupervoxelID,
		Status:       statusNoSupervoxel,
	}
	if strings.TrimSpace(row.SupervoxelID) == "" {
		return status
	}

	supervoxelID, err := strconv.ParseUint(strings.TrimSpace(row.SupervoxelID), 10, 64)
	if err != nil {
		status.Status = statusError
		status.Error = fmt.Sprintf("invalid supervoxel_id %q: %v", row.SupervoxelID, err)
		return status
	}

	rootID, err := strconv.ParseUint(strings.TrimSpace(row.RootID), 10, 64)
	if err != nil {
		status.Status = statusError
		status.Error = fmt.Sprintf("invalid root_id %q: %v", row.RootID, err)
		return status
	}

	currentRootID, err := client.GetRootID(supervoxelID)
	if err != nil {
		status.Status = statusError
		status.Error = err.Error()
		return status
	}

	status.CurrentRootID = strconv.FormatUint(currentRootID, 10)
	if rootID == currentRootID {
		status.Status = statusOK
	} else {
		status.Status = statusStale
	}
	return status
}

func fetchNearestColumn(data rootInfoDataSource, row *seatable.NeuronInfoRow) (*rootInfoNearestColumn, string, error) {
	regionOpts, err := data.RegionOptions()
	if err != nil {
		return nil, "", err
	}

	candidates, err := data.QueryEPGPEGPositions(regionOpts)
	if err != nil {
		return nil, "", err
	}
	if len(candidates) == 0 {
		return nil, "no EPG/PEG neurons found", nil
	}

	var closest *seatable.NeuronPositionRow
	closestDist := math.MaxFloat64
	for i := range candidates {
		candidate := &candidates[i]
		if !candidate.HasPosition() {
			continue
		}
		d := euclideanDistance(row.X, row.Y, row.Z, candidate.X, candidate.Y, candidate.Z)
		if d < closestDist {
			closestDist = d
			closest = candidate
		}
	}
	if closest == nil {
		return nil, "no EPG/PEG neurons with valid position coordinates found", nil
	}

	return &rootInfoNearestColumn{
		Region:       closest.Region,
		RootID:       closest.RootID,
		Side:         closest.Side,
		Distance:     closestDist,
		SideRelation: nearestSideRelation(row.Side, closest.Side),
	}, "", nil
}

func nearestSideRelation(targetSide, nearestSide string) string {
	targetSide = normalizeSide(targetSide)
	nearestSide = normalizeSide(nearestSide)
	if targetSide == "" || nearestSide == "" {
		return "unknown"
	}
	if targetSide == nearestSide {
		return "same"
	}
	return "different"
}

func summarizeRootInfoHistory(rows []cave.ChangeLogRow, filtered bool, limit int) rootInfoHistory {
	entries := caveRowsToHistoryEntries(rows)
	history := rootInfoHistory{
		Filtered: filtered,
		Total:    len(entries),
		Entries:  []caveHistoryEntry{},
	}

	for i := range entries {
		if entries[i].IsMerge {
			history.Merges++
		} else {
			history.Splits++
		}
		if history.Latest == nil || entries[i].Timestamp > history.Latest.Timestamp ||
			(entries[i].Timestamp == history.Latest.Timestamp && entries[i].OperationID > history.Latest.OperationID) {
			entry := entries[i]
			history.Latest = &entry
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Timestamp != entries[j].Timestamp {
			return entries[i].Timestamp > entries[j].Timestamp
		}
		return entries[i].OperationID > entries[j].OperationID
	})
	if limit > len(entries) {
		limit = len(entries)
	}
	if limit > 0 {
		history.Entries = entries[:limit]
	}

	return history
}

func writeRootInfoText(w io.Writer, result *rootInfoResult) error {
	write := func(format string, args ...interface{}) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}

	if err := writeRootInfoRootSection(write, result); err != nil {
		return err
	}
	if err := writeRootInfoClassificationSection(write, result.Classification); err != nil {
		return err
	}
	if err := writeRootInfoPositionSection(write, result); err != nil {
		return err
	}
	if err := writeRootInfoNearestColumnSection(write, result); err != nil {
		return err
	}
	if err := writeRootInfoHistorySection(write, result.History); err != nil {
		return err
	}
	return writeRootInfoExtraFieldsSection(write, result.ExtraFields)
}

type rootInfoTextWriter func(format string, args ...interface{}) error

func writeRootInfoRootSection(write rootInfoTextWriter, result *rootInfoResult) error {
	if err := write("root\n"); err != nil {
		return err
	}
	if err := write("  root_id: %s\n", result.RootID); err != nil {
		return err
	}
	if err := write("  status: %s\n", result.CAVE.Status); err != nil {
		return err
	}
	if result.CAVE.CurrentRootID != "" {
		if err := write("  current_root: %s\n", result.CAVE.CurrentRootID); err != nil {
			return err
		}
	}
	if result.CAVE.SupervoxelID != "" {
		if err := write("  supervoxel_id: %s\n", result.CAVE.SupervoxelID); err != nil {
			return err
		}
	}
	if result.CAVE.Error != "" {
		if err := write("  cave_error: %s\n", result.CAVE.Error); err != nil {
			return err
		}
	}

	return nil
}

func writeRootInfoClassificationSection(write rootInfoTextWriter, classification rootInfoClassification) error {
	if err := write("\nclassification\n"); err != nil {
		return err
	}
	fields := []struct {
		name  string
		value string
	}{
		{"super_class", classification.SuperClass},
		{"cell_class", classification.CellClass},
		{"cell_type", classification.CellType},
		{"cell_subtype", classification.CellSubtype},
		{"side", classification.Side},
		{"region", classification.Region},
		{"tract", classification.Tract},
		{"nerve", classification.Nerve},
		{"hemilineage", classification.Hemilineage},
		{"proofread", classification.Proofread},
	}
	for _, field := range fields {
		if err := write("  %s: %s\n", field.name, displayValue(field.value)); err != nil {
			return err
		}
	}

	return nil
}

func writeRootInfoPositionSection(write rootInfoTextWriter, result *rootInfoResult) error {
	if err := write("\nposition\n"); err != nil {
		return err
	}
	if result.Position != nil {
		if err := write("  xyz: %g, %g, %g\n", result.Position.X, result.Position.Y, result.Position.Z); err != nil {
			return err
		}
	} else {
		if err := write("  unavailable: %s\n", displayValue(result.PositionError)); err != nil {
			return err
		}
	}

	return nil
}

func writeRootInfoNearestColumnSection(write rootInfoTextWriter, result *rootInfoResult) error {
	if err := write("\nnearest_column\n"); err != nil {
		return err
	}
	if result.NearestColumn != nil {
		nearest := result.NearestColumn
		if err := write("  region: %s\n", displayValue(nearest.Region)); err != nil {
			return err
		}
		if err := write("  root_id: %s\n", displayValue(nearest.RootID)); err != nil {
			return err
		}
		if err := write("  side: %s\n", displayValue(nearest.Side)); err != nil {
			return err
		}
		if err := write("  distance: %.3f\n", nearest.Distance); err != nil {
			return err
		}
		if err := write("  side_relation: %s\n", displayValue(nearest.SideRelation)); err != nil {
			return err
		}
	} else if err := write("  unavailable: %s\n", displayValue(result.NearestError)); err != nil {
		return err
	}

	return nil
}

func writeRootInfoHistorySection(write rootInfoTextWriter, history rootInfoHistory) error {
	if err := write("\ncave_history\n"); err != nil {
		return err
	}
	if history.Error != "" {
		return write("  unavailable: %s\n", history.Error)
	}
	if err := write("  edits: %d\n", history.Total); err != nil {
		return err
	}
	if err := write("  merges: %d\n", history.Merges); err != nil {
		return err
	}
	if err := write("  splits: %d\n", history.Splits); err != nil {
		return err
	}
	if history.Latest != nil {
		latest := history.Latest
		if err := write("  latest: %s %s by %s\n", latest.TimestampUTC, latest.Type, displayValue(latest.UserName)); err != nil {
			return err
		}
	}
	if len(history.Entries) > 0 {
		if err := write("  recent:\n"); err != nil {
			return err
		}
		for _, entry := range history.Entries {
			if err := write("    %s\t%s\toperation=%d\tbefore=%s\tafter=%s\tuser=%s\n",
				entry.TimestampUTC,
				entry.Type,
				entry.OperationID,
				formatHistoryRootList(entry.BeforeRootIDs),
				formatHistoryRootList(entry.AfterRootIDs),
				displayValue(entry.UserName),
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeRootInfoExtraFieldsSection(write rootInfoTextWriter, extraFields map[string]string) error {
	if len(extraFields) == 0 {
		return nil
	}

	if err := write("\nextra_fields\n"); err != nil {
		return err
	}
	keys := make([]string, 0, len(extraFields))
	for key := range extraFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := write("  %s: %s\n", key, extraFields[key]); err != nil {
			return err
		}
	}
	return nil
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
