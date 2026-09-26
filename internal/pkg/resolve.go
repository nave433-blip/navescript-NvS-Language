package pkg

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Resolve maps an import spec to an absolute file path in the package cache.
// It is the function the evaluator's PackageResolver hook calls:
//
//	import "name"          -> <cache>/name@<ver>/<main from manifest>
//	import "name/sub.nvs"  -> <cache>/name@<ver>/sub.nvs
//
// It returns ok=false when the package is not installed or the file is missing.
//
// Version selection: the nvs.lock in the current directory wins (reproducible).
// Without a lockfile entry, a single installed version wins; with several,
// the highest version is picked. This fallback is documented and deterministic.
func Resolve(spec string) (filePath string, ok bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	name, sub := splitImportSpec(spec)
	dir, found := findCachedDir(name)
	if !found {
		return "", false
	}
	var rel string
	if sub == "" {
		m, err := Load(dir)
		if err != nil {
			return "", false
		}
		rel = m.Main
		if rel == "" {
			rel = "main.nvs"
		}
	} else {
		rel = sub
	}
	abs := filepath.Join(dir, rel)
	fi, err := os.Stat(abs)
	if err != nil || fi.IsDir() {
		return "", false
	}
	return abs, true
}

// splitImportSpec splits "name" -> ("name","") and "name/sub/path.nvs" ->
// ("name","sub/path.nvs").
func splitImportSpec(spec string) (name, sub string) {
	if i := strings.Index(spec, "/"); i >= 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

// findCachedDir locates the cache directory for a package name.
func findCachedDir(name string) (string, bool) {
	root := cacheRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	prefix := name + "@"
	var candidates []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	// 1. Lockfile in the current directory pins version+commit.
	if wd, err := os.Getwd(); err == nil {
		if lf, err := readLock(wd); err == nil {
			if le, ok := lf.Packages[name]; ok {
				if dir := matchCandidate(candidates, le); dir != "" {
					return filepath.Join(root, dir), true
				}
			}
		}
	}
	// 2. Single installed version wins.
	if len(candidates) == 1 {
		return filepath.Join(root, candidates[0]), true
	}
	// 3. Deterministic fallback: highest version first.
	sort.Slice(candidates, func(i, j int) bool {
		return compareVersionSuffix(candidates[i], candidates[j]) > 0
	})
	return filepath.Join(root, candidates[0]), true
}

// matchCandidate picks the cache dir matching a lock entry's version/commit.
func matchCandidate(candidates []string, le LockEntry) string {
	want := []string{}
	if le.Version != "" && le.Commit != "" {
		want = append(want, le.Version+"-"+shortCommit(le.Commit))
	}
	if le.Version != "" {
		want = append(want, le.Version)
	}
	if le.Commit != "" {
		want = append(want, shortCommit(le.Commit))
	}
	for _, dir := range candidates {
		key := dir[strings.LastIndex(dir, "@")+1:]
		for _, w := range want {
			if key == w {
				return dir
			}
		}
		// Commit-prefix match: a lockfile commit matches a longer key.
		if le.Commit != "" && strings.HasPrefix(le.Commit, key) {
			return dir
		}
	}
	return ""
}

// compareVersionSuffix orders "name@1.2.3-x" style dir names by version,
// then by key, so the newest version sorts last. Non-semver keys sort
// below semver ones.
func compareVersionSuffix(a, b string) int {
	ka := a[strings.LastIndex(a, "@")+1:]
	kb := b[strings.LastIndex(b, "@")+1:]
	va := semverParts(ka)
	vb := semverParts(kb)
	if va == nil && vb == nil {
		return strings.Compare(ka, kb)
	}
	if va == nil {
		return -1
	}
	if vb == nil {
		return 1
	}
	for i := 0; i < 3; i++ {
		if va[i] != vb[i] {
			if va[i] < vb[i] {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(ka, kb)
}

// semverParts extracts leading numeric major.minor.patch from a cache key,
// or nil when the key does not start with one.
func semverParts(key string) []int {
	num := ""
	dots := 0
	for _, r := range key {
		switch {
		case r >= '0' && r <= '9':
			num += string(r)
		case r == '.' && dots < 2:
			num += "."
			dots++
		default:
			goto done
		}
	}
done:
	parts := strings.Split(num, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n := 0
		for _, r := range p {
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out
}
