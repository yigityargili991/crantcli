// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parsePos parses an x,y,z position such as "100.5,200.3,300.1". Coordinates
// that are not finite numbers are rejected.
func parsePos(s string) (float64, float64, float64, error) {
	posErr := fmt.Errorf("invalid --pos format; expected x,y,z (e.g. 100.5,200.3,300.1)")
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return 0, 0, 0, posErr
	}
	var coords [3]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, 0, 0, posErr
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, 0, 0, posErr
		}
		coords[i] = f
	}
	return coords[0], coords[1], coords[2], nil
}

// euclideanDistance returns the 3D Euclidean distance between two points.
func euclideanDistance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	dz := z1 - z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
