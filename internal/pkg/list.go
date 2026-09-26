package pkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CachedPackage describes one package directory in the cache.
type CachedPackage struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	Commit      string `json:"commit"`
	Dir         string `json:"dir"`
	InstalledAt string `json:"installed_at,omitempty"`
}

// List returns every package installed in the cache, sorted by name.
func List() ([]CachedPackage, error) {
	root := cacheRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list: cannot read cache %s: %w", root, err)
	}
	var out []CachedPackage
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		cp := CachedPackage{Dir: dir}
		if data, err := os.ReadFile(filepath.Join(dir, ".nvs-meta.json")); err == nil {
			var meta cacheMeta
			if json.Unmarshal(data, &meta) == nil {
				cp.Name = meta.Name
				cp.Version = meta.Version
				cp.Source = meta.Source
				cp.Commit = meta.Commit
				cp.InstalledAt = meta.InstalledAt
			}
		}
		if cp.Name == "" {
			// Fall back to the directory name when meta is missing.
			if i := strings.LastIndex(e.Name(), "@"); i >= 0 {
				cp.Name = e.Name()[:i]
				cp.Version = e.Name()[i+1:]
			} else {
				cp.Name = e.Name()
			}
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// Remove deletes every cached version of name and drops it from the current
// directory's nvs.lock when present. It fails loudly when nothing is installed.
func Remove(name string) error {
	root := cacheRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("remove: cannot read cache %s: %w", root, err)
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), name+"@") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove: cannot delete %s: %w", dir, err)
		}
		removed++
	}
	if removed == 0 {
		return fmt.Errorf("remove: package %q is not installed", name)
	}
	if wd, err := os.Getwd(); err == nil {
		if lf, err := readLock(wd); err == nil {
			if _, ok := lf.Packages[name]; ok {
				delete(lf.Packages, name)
				if err := writeLock(wd, lf); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// PublishCheck validates that dir is ready to publish and returns the manual
// publishing steps. There is no central NvS registry — GitHub IS the registry —
// so this never uploads anything; it just tells the user exactly what to do.
func PublishCheck(dir string) (string, error) {
	m, err := Load(dir)
	if err != nil {
		return "", fmt.Errorf("publish: %w", err)
	}
	main := m.Main
	if main == "" {
		main = "main.nvs"
	}
	if _, err := os.Stat(filepath.Join(dir, main)); err != nil {
		return "", fmt.Errorf("publish: manifest main %q does not exist in %s", main, dir)
	}
	if m.Description == "" {
		return "", fmt.Errorf(`publish: field "description" is empty; write one before publishing`)
	}
	steps := fmt.Sprintf(`Package %s@%s passed checks (manifest valid, %s exists).

There is no central NvS registry — GitHub is the registry.
Publish manually:

  cd %s
  git add -A
  git commit -m "release %s"
  git tag %s            # tag must match the manifest version
  git push origin main --tags

Others then install it with:

  nvs pkg install <you>/%s@%s

Keep the manifest version and the git tag in sync on every release.
`, m.Name, m.Version, main, dir, m.Version, m.Version, m.Name, m.Version)
	return steps, nil
}
