package pkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LockEntry records one resolved package in nvs.lock.
type LockEntry struct {
	Version string `json:"version"` // version from the package's manifest
	Source  string `json:"source"`  // original spec source: "user/repo" or a local path
	Commit  string `json:"commit"`  // full git commit for github sources, "" for local
}

// LockFile is the on-disk nvs.lock format.
type LockFile struct {
	Version  int                  `json:"version"`
	Packages map[string]LockEntry `json:"packages"`
}

const lockFormatVersion = 1

// readLock loads dir/nvs.lock; a missing file yields an empty lock (not an error).
func readLock(dir string) (*LockFile, error) {
	lf := &LockFile{Version: lockFormatVersion, Packages: map[string]LockEntry{}}
	data, err := os.ReadFile(filepath.Join(dir, LockFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return lf, nil
		}
		return nil, fmt.Errorf("cannot read %s: %w", LockFileName, err)
	}
	if err := json.Unmarshal(data, lf); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", LockFileName, err)
	}
	if lf.Packages == nil {
		lf.Packages = map[string]LockEntry{}
	}
	return lf, nil
}

// writeLock persists the lockfile to dir/nvs.lock.
func writeLock(dir string, lf *LockFile) error {
	lf.Version = lockFormatVersion
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode %s: %w", LockFileName, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, LockFileName), data, 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", LockFileName, err)
	}
	return nil
}

// mergeLock adds entries into dir/nvs.lock (creating it when absent) without
// dropping existing entries.
func mergeLock(dir string, add map[string]LockEntry) error {
	lf, err := readLock(dir)
	if err != nil {
		return err
	}
	for name, e := range add {
		lf.Packages[name] = e
	}
	return writeLock(dir, lf)
}
