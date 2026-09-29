package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"crantcli/internal/config"
	"crantcli/internal/nglstate"
	"crantcli/internal/textout"
)

func deliverCaveHistoryState(errOut io.Writer, results []caveHistoryResult, opts caveHistoryOptions) error {
	state, layers, info, err := buildCaveHistoryState(results, opts.HistoryLimit, opts.Compact)
	if err != nil {
		return err
	}
	for _, result := range results {
		if len(result.Entries) == 0 {
			fmt.Fprintf(errOut, "No history found for %s; showing the queried root.\n", result.RootID)
		} else if opts.HistoryLimit > 0 && len(result.Entries) > opts.HistoryLimit {
			fmt.Fprintf(errOut, "Showing latest %d of %d edits for %s; use --history-limit 0 for all.\n", opts.HistoryLimit, len(result.Entries), result.RootID)
		}
	}
	if opts.Labels {
		if err := caveHistoryAttachLabels(errOut, layers, info, "edit history (editor and UTC time)", opts.LabelsTTL, resolveLabelsHook(opts.LabelsHook)); err != nil {
			return err
		}
	} else if opts.Compact {
		fmt.Fprintln(errOut, "Use --labels to show edit, editor, and UTC time beside roots in the history layer's Seg. panel.")
	}
	return caveHistoryDeliverState(&nglstate.LoadResult{State: state, Source: nglstate.SourceTemplate}, nglstate.DeliveryOptions{Open: opts.Open, OutputFile: opts.Output})
}

// Historical roots retain their original geometry. Compact mode uses meshes
// without the graph, so overlapping historical roots remain independent.
// The full view isolates each stage's graph in a separate layer for 2D overlays.
func buildCaveHistoryState(results []caveHistoryResult, limit int, compact bool) (map[string]interface{}, []map[string]interface{}, []byte, error) {
	var state map[string]interface{}
	if err := json.Unmarshal(nglstate.DefaultScene, &state); err != nil {
		return nil, nil, nil, fmt.Errorf("loading history scene: %w", err)
	}
	// Retain the dataset's camera and coordinate system, with a clean layer list.
	state["layers"] = []interface{}{map[string]interface{}{
		"name": "aligned", "type": "image", "source": config.ImageSource,
	}}
	delete(state, "selectedLayer")

	specs := planCaveHistoryLayers(results, limit)
	labels := caveHistoryRootLabels(specs)
	ids := sortedCaveHistoryRootIDs(labels)
	var historyLayers []map[string]interface{}
	if compact {
		historyLayers = buildCompactCaveHistoryLayers(specs, ids)
		if len(historyLayers) > 0 {
			state["selectedLayer"] = map[string]interface{}{"layer": "Edit history", "visible": true}
		}
	} else {
		historyLayers = buildCaveHistoryLayers(specs)
	}
	colorCaveHistoryLayers(historyLayers, ids)
	for _, layer := range historyLayers {
		state["layers"] = append(state["layers"].([]interface{}), layer)
	}
	info, err := marshalCaveHistoryLabels(ids, labels)
	return state, historyLayers, info, err
}

type caveHistoryLayerSpec struct {
	name    string
	ids     []string
	visible bool
	label   string
}

// buildCaveHistoryLayers creates independent segmentation layers for 2D/3D
// before/after comparisons.
func buildCaveHistoryLayers(specs []caveHistoryLayerSpec) []map[string]interface{} {
	var historyLayers []map[string]interface{}
	for _, spec := range specs {
		if len(spec.ids) == 0 {
			continue
		}
		layer := map[string]interface{}{
			"name": spec.name, "type": "segmentation", "source": config.SegmentationSource,
			"tab": "segments", "visible": spec.visible, "selectedAlpha": 0.4,
			"objectAlpha": 1.0,
		}
		nglstate.AddSegments(layer, spec.ids, false)
		historyLayers = append(historyLayers, layer)
	}
	return historyLayers
}

// buildCompactCaveHistoryLayers keeps every root in the Seg. panel while only
// showing the latest before stage. Neuroglancer's ! prefix means starred but
// hidden. Enabling only the mesh subsource avoids a shared graph remapping
// overlapping historical roots and prevents misleading 2D segmentation.
func buildCompactCaveHistoryLayers(specs []caveHistoryLayerSpec, ids []string) []map[string]interface{} {
	if len(ids) == 0 {
		return nil
	}
	visible := make(map[string]bool)
	for _, spec := range specs {
		if spec.visible {
			for _, id := range spec.ids {
				visible[id] = true
			}
		}
	}
	segments := make([]interface{}, len(ids))
	for i, id := range ids {
		if !visible[id] {
			id = "!" + id
		}
		segments[i] = id
	}
	return []map[string]interface{}{{
		"name": "Edit history", "type": "segmentation", "tab": "segments",
		"source": map[string]interface{}{
			"url": config.SegmentationSource, "enableDefaultSubsources": false,
			"subsources": map[string]interface{}{"mesh": true},
		},
		"segments": segments, "visible": true, "objectAlpha": 1.0,
	}}
}

// caveHistoryRootLabels collects each root's distinct labels in stage order.
func caveHistoryRootLabels(specs []caveHistoryLayerSpec) map[string][]string {
	labels := make(map[string][]string)
	for _, spec := range specs {
		for _, id := range spec.ids {
			if !slices.Contains(labels[id], spec.label) {
				labels[id] = append(labels[id], spec.label)
			}
		}
	}
	return labels
}

// planCaveHistoryLayers selects edits once per queried root and determines the
// names, labels, and initial visibility of their before/after stages.
func planCaveHistoryLayers(results []caveHistoryResult, limit int) []caveHistoryLayerSpec {
	var specs []caveHistoryLayerSpec
	seenRoots := make(map[string]bool)
	for _, result := range results {
		if seenRoots[result.RootID] {
			continue
		}
		seenRoots[result.RootID] = true
		entries := latestCaveHistoryEntries(result.Entries, limit)
		specs = append(specs, caveHistoryLayerSpec{
			name: "Root " + result.RootID + " | queried", ids: []string{result.RootID},
			visible: len(entries) == 0, label: "Queried root " + result.RootID,
		})
		for i, entry := range entries {
			who := strings.TrimSpace(textout.Sanitize(entry.UserName))
			if who == "" {
				who = fmt.Sprintf("user %d", entry.UserID)
			}
			event := fmt.Sprintf("%s #%d | %s | %s", entry.Type, entry.OperationID, who, entry.TimestampUTC)
			for _, stage := range []struct {
				name string
				ids  []string
			}{
				{"before", entry.BeforeRootIDs}, {"after (reported)", entry.AfterRootIDs},
			} {
				visible := i == 0 && (stage.name == "before" || len(entry.BeforeRootIDs) == 0)
				specs = append(specs, caveHistoryLayerSpec{
					name: "Root " + result.RootID + " | " + event + " | " + stage.name,
					ids:  stage.ids, visible: visible, label: stage.name + " " + event,
				})
			}
		}
	}
	return specs
}

// latestCaveHistoryEntries sorts and limits a copy, preserving the input order.
func latestCaveHistoryEntries(entries []caveHistoryEntry, limit int) []caveHistoryEntry {
	entries = append([]caveHistoryEntry(nil), entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Timestamp != entries[j].Timestamp {
			return entries[i].Timestamp > entries[j].Timestamp
		}
		return entries[i].OperationID > entries[j].OperationID
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func sortedCaveHistoryRootIDs(labels map[string][]string) []string {
	ids := make([]string, 0, len(labels))
	for id := range labels {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if len(ids[i]) != len(ids[j]) {
			return len(ids[i]) < len(ids[j])
		}
		return ids[i] < ids[j]
	})
	return ids
}

// colorCaveHistoryLayers assigns colors once so a root keeps its color across
// all stages/layers.
func colorCaveHistoryLayers(historyLayers []map[string]interface{}, ids []string) {
	groups := make([][]string, len(ids))
	for i, id := range ids {
		groups[i] = []string{id}
	}
	colorLayer := make(map[string]interface{})
	nglstate.SetSegmentColorByGroupValues(colorLayer, groups, "colored")
	colors, _ := colorLayer["segmentColors"].(map[string]interface{})
	for _, layer := range historyLayers {
		for _, segment := range layer["segments"].([]interface{}) {
			id := strings.TrimPrefix(segment.(string), "!")
			nglstate.SetSegmentColor(layer, []string{id}, colors[id].(string))
		}
	}
}

func marshalCaveHistoryLabels(ids []string, labels map[string][]string) ([]byte, error) {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = strings.Join(labels[id], "; ")
	}
	return json.Marshal(map[string]interface{}{
		"@type": "neuroglancer_segment_properties",
		"inline": map[string]interface{}{
			"ids":        ids,
			"properties": []interface{}{map[string]interface{}{"id": "label", "type": "label", "values": values}},
		},
	})
}
