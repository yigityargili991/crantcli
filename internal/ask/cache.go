package ask

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type vocabCache struct {
	Fetched time.Time           `json:"fetched"`
	Values  map[string][]string `json:"values"`
}

// LoadVocab returns the table vocabulary cached at path while it is younger
// than maxAge. Otherwise it fetches the values, rewrites the cache, and falls
// back to an older cache only when fetching fails.
func LoadVocab(path string, maxAge time.Duration, fetch func() (map[string][]string, error)) (*Vocab, error) {
	var cached vocabCache
	data, readErr := os.ReadFile(path)
	if readErr == nil && json.Unmarshal(data, &cached) == nil && len(cached.Values) > 0 &&
		time.Since(cached.Fetched) < maxAge {
		return NewVocab(cached.Values), nil
	}
	values, err := fetch()
	if err != nil {
		if len(cached.Values) > 0 {
			return NewVocab(cached.Values), nil
		}
		return nil, fmt.Errorf("fetching table values: %w", err)
	}
	if err := writeVocabCache(path, vocabCache{Fetched: time.Now().UTC(), Values: values}); err != nil {
		return nil, err
	}
	return NewVocab(values), nil
}

func writeVocabCache(path string, c vocabCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating ask cache directory: %w", err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing table value cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("writing table value cache: %w", err)
	}
	return nil
}
