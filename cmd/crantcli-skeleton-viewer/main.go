package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"crantcli/internal/skeleton"
	"crantcli/internal/skeletonviewer"
)

func main() {
	projectionName := flag.String("projection", string(skeleton.Projection3D), "View: 3d, xy, xz, yz, or iso")
	maxNodes := flag.Int("max-nodes", 0, "Maximum nodes to render; 0 keeps all nodes")
	infoPath := flag.String("info", "", "Optional viewer info JSON file")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: crantcli-skeleton-viewer [--projection 3d|xy|xz|yz|iso] [--max-nodes N] <skeleton.json>")
		os.Exit(2)
	}
	projection, err := skeleton.ParseProjection(*projectionName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	limit, err := skeleton.ParseMaxNodes(*maxNodes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading skeleton JSON: %v\n", err)
		os.Exit(1)
	}
	var sk skeleton.Skeleton
	if err := json.Unmarshal(data, &sk); err != nil {
		fmt.Fprintf(os.Stderr, "decoding skeleton JSON: %v\n", err)
		os.Exit(1)
	}
	if err := skeleton.ValidateSkeleton(&sk); err != nil {
		fmt.Fprintf(os.Stderr, "validating skeleton JSON: %v\n", err)
		os.Exit(1)
	}
	infoLines, err := readViewerInfo(*infoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading viewer info: %v\n", err)
		os.Exit(1)
	}
	if err := skeletonviewer.Run(skeleton.LimitSkeleton(&sk, limit), skeletonviewer.Options{Projection: projection, InfoLines: infoLines}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readViewerInfo(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info skeleton.ViewerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	if info.Error != "" {
		return []string{"root_info", "unavailable: " + info.Error}, nil
	}
	return info.Lines, nil
}
