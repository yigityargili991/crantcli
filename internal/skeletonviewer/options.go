package skeletonviewer

import "crantcli/internal/skeleton"

type Options struct {
	Projection skeleton.Projection
	InfoLines  []string
}
