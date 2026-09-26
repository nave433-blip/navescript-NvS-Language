// Package pkg is the nvs package manager engine ("nvs pkg").
//
// It implements manifests (nvs.json), installing packages from GitHub
// (git clone) or local paths into a content cache, transitive dependency
// resolution with cycle detection, an nvs.lock lockfile for reproducible
// installs, and Resolve — the hook the evaluator calls to turn
// `import "name"` / `import "name/sub.nvs"` into absolute file paths.
//
// Honest scope: there is NO central NvS registry. GitHub is the registry;
// PublishCheck prints manual git tag/push steps and never uploads anywhere.
//
// Cache layout: <cacheRoot>/<name>@<version-or-commit>/, where cacheRoot is
// $NVS_PKG_CACHE when set, otherwise the exported CacheRoot variable, which
// defaults to ~/.nvs/packages. Tests point it at t.TempDir() so they never
// touch the real cache.
package pkg

import (
	"os"
	"path/filepath"
)

// CacheRoot is the default package cache directory. It is consulted only
// when the NVS_PKG_CACHE environment variable is unset, so library users
// and tests can override either one.
var CacheRoot = defaultCacheRoot()

func defaultCacheRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".nvs", "packages")
}

// cacheRoot returns the effective cache directory: $NVS_PKG_CACHE wins,
// then the CacheRoot variable.
func cacheRoot() string {
	if v := os.Getenv("NVS_PKG_CACHE"); v != "" {
		return v
	}
	return CacheRoot
}

// ManifestFileName is the package manifest file every nvs package carries.
const ManifestFileName = "nvs.json"

// LockFileName is the lockfile written into the working directory after install.
const LockFileName = "nvs.lock"
