package skeleton

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Cache struct {
	root string
}

func DefaultCacheRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "crantcli"), nil
}

func NewCache(root string) Cache {
	return Cache{root: root}
}

func (c Cache) SkeletonPath(rootID string) string {
	return filepath.Join(c.root, "skeletons", rootID+".json")
}

func (c Cache) ViewerInfoPath(rootID string) string {
	return filepath.Join(c.root, "skeleton-info", rootID+".json")
}

func (c Cache) BridgeDir() string {
	return filepath.Join(c.root, "skeleton-bridge")
}

func (c Cache) ReadSkeleton(rootID string) (*Skeleton, bool, error) {
	path := c.SkeletonPath(rootID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading skeleton cache: %w", err)
	}
	var sk Skeleton
	if err := json.Unmarshal(data, &sk); err != nil {
		return nil, false, fmt.Errorf("decoding skeleton cache: %w", err)
	}
	if err := ValidateSkeleton(&sk); err != nil {
		return nil, false, fmt.Errorf("invalid skeleton cache: %w", err)
	}
	if sk.RootID != rootID {
		return nil, false, nil
	}
	return &sk, true, nil
}

func (c Cache) ReadViewerInfo(rootID string) (ViewerInfo, bool, error) {
	path := c.ViewerInfoPath(rootID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ViewerInfo{}, false, nil
		}
		return ViewerInfo{}, false, fmt.Errorf("reading skeleton info cache: %w", err)
	}
	var info ViewerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ViewerInfo{}, false, fmt.Errorf("decoding skeleton info cache: %w", err)
	}
	if info.RootID != rootID {
		return ViewerInfo{}, false, nil
	}
	if info.Error != "" {
		return info, false, nil
	}
	return info, true, nil
}

func (c Cache) WriteSkeleton(rootID string, sk *Skeleton) error {
	if sk == nil {
		return nil
	}
	if sk.RootID != "" && sk.RootID != rootID {
		return fmt.Errorf("skeleton root_id %q does not match requested root_id %q", sk.RootID, rootID)
	}
	sk.RootID = rootID
	path := c.SkeletonPath(rootID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating skeleton cache directory: %w", err)
	}
	data, err := json.MarshalIndent(sk, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding skeleton cache: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".skeleton-*.json")
	if err != nil {
		return fmt.Errorf("creating skeleton cache temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("writing skeleton cache temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing skeleton cache temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("storing skeleton cache: %w", err)
	}
	return nil
}

type ViewerInfo struct {
	RootID string   `json:"root_id"`
	Lines  []string `json:"lines,omitempty"`
	Error  string   `json:"error,omitempty"`
}

func (c Cache) WriteViewerInfo(info ViewerInfo) error {
	path := c.ViewerInfoPath(info.RootID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating skeleton info cache directory: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding skeleton info: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".skeleton-info-*.json")
	if err != nil {
		return fmt.Errorf("creating skeleton info temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("writing skeleton info temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing skeleton info temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("storing skeleton info: %w", err)
	}
	return nil
}
