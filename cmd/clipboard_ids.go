// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import "strings"

// parseIDs splits clipboard text into individual root IDs.
// Accepts whitespace, newlines, commas, or any mix as separators.
func parseIDs(text string) []string {
	// Replace commas with spaces so we can split uniformly
	text = strings.ReplaceAll(text, ",", " ")
	parts := strings.Fields(text)

	seen := make(map[string]bool, len(parts))
	var ids []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		ids = append(ids, p)
	}
	return ids
}
