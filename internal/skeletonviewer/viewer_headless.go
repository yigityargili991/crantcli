//go:build headless

package skeletonviewer

import (
	"fmt"

	"crantcli/internal/skeleton"
)

func Run(sk *skeleton.Skeleton, _ Options) error {
	if err := skeleton.ValidateSkeleton(sk); err != nil {
		return err
	}
	return fmt.Errorf("skeleton viewer is unavailable in headless builds")
}
