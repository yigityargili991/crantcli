package cmd

import (
	"strconv"
	"strings"
	"time"

	"crantcli/internal/cave"
)

type caveHistoryEntry struct {
	OperationID     uint64   `json:"operation_id"`
	Timestamp       int64    `json:"timestamp"`
	TimestampUTC    string   `json:"timestamp_utc"`
	UserID          uint64   `json:"user_id"`
	BeforeRootIDs   []string `json:"before_root_ids"`
	AfterRootIDs    []string `json:"after_root_ids"`
	IsMerge         bool     `json:"is_merge"`
	Type            string   `json:"type"`
	UserName        string   `json:"user_name"`
	UserAffiliation string   `json:"user_affiliation"`
}

func caveRowsToHistoryEntries(rows []cave.ChangeLogRow) []caveHistoryEntry {
	entries := make([]caveHistoryEntry, len(rows))
	for i, row := range rows {
		editType := "split"
		if row.IsMerge {
			editType = "merge"
		}

		entries[i] = caveHistoryEntry{
			OperationID:     row.OperationID,
			Timestamp:       row.Timestamp,
			TimestampUTC:    formatCaveHistoryTimestamp(row.Timestamp),
			UserID:          row.UserID,
			BeforeRootIDs:   uint64sToStrings(row.BeforeRootIDs),
			AfterRootIDs:    uint64sToStrings(row.AfterRootIDs),
			IsMerge:         row.IsMerge,
			Type:            editType,
			UserName:        row.UserName,
			UserAffiliation: row.UserAffiliation,
		}
	}
	return entries
}

func formatCaveHistoryTimestamp(timestampMS int64) string {
	return time.UnixMilli(timestampMS).UTC().Format(time.RFC3339)
}

func uint64sToStrings(ids []uint64) []string {
	if len(ids) == 0 {
		return []string{}
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.FormatUint(id, 10)
	}
	return out
}

func formatHistoryRootList(ids []string) string {
	if len(ids) == 0 {
		return "-"
	}
	return strings.Join(ids, ",")
}
