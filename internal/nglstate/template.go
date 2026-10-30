// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package nglstate

import _ "embed"

//go:embed crant_scene.json
var DefaultScene []byte

// TemplateSecondarySourceURLs returns the URLs of the sources the built-in
// template adds after the primary one on its segmentation layers. Today that is
// the label source it ships for its own segments, which no manifest tracks.
func TemplateSecondarySourceURLs() []string {
	scene, err := parseJSON(DefaultScene)
	if err != nil {
		return nil
	}
	return secondarySourceURLs(scene)
}
