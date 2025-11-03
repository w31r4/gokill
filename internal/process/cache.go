package process

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func cachePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "gkill_cache.json"), nil
}

// Save saves the process list to the cache.
func Save(processes []*Item) error {
	path, err := cachePath()
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(processes)
}

// Load loads the process list from the cache.
func Load() ([]*Item, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var processes []*Item
	if err := json.NewDecoder(f).Decode(&processes); err != nil {
		return nil, err
	}
	return processes, nil
}
