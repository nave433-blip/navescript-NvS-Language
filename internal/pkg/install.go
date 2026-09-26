package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InstallOptions controls Install.
type InstallOptions struct {
	// WorkDir is where nvs.lock is written. Empty means the current directory.
	WorkDir string
}

// cacheMeta is written into every installed package dir so List/Resolve can
// report where it came from without a lockfile.
type cacheMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	Commit      string `json:"commit"`
	InstalledAt string `json:"installed_at"`
}

// Installed describes one package placed in the cache.
type Installed struct {
	Name    string
	Version string
	Commit  string
	Dir     string // cache directory
	Main    string // absolute path of the package entry file
}

// Install installs the package named by spec (user/repo, user/repo@tag, or a
// local path), including its transitive dependencies, into the package cache.
// It records every installed package in WorkDir/nvs.lock so the install is
// reproducible via InstallFromLock.
//
// GitHub specs are fetched with the git CLI (os/exec): the call fails loudly,
// including git's stderr, when git is missing or the clone fails.
func Install(spec string, opts InstallOptions) (*Installed, error) {
	ps, err := parseSpec(spec)
	if err != nil {
		return nil, err
	}
	workDir := opts.WorkDir
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("install: cannot determine working directory: %w", err)
		}
		workDir = wd
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("install: bad work dir: %w", err)
	}

	added := map[string]LockEntry{}
	inst, err := installOne(ps, "", workDir, nil, added)
	if err != nil {
		return nil, err
	}
	if err := mergeLock(workDir, added); err != nil {
		return nil, err
	}
	return inst, nil
}

// installOne installs a single parsed spec. baseDir is the directory relative
// local dep specs resolve against ("" = current directory). stack holds the
// in-progress dependency chain for cycle detection. Lock entries for every
// package installed along the way accumulate in added.
func installOne(ps parsedSpec, baseDir, workDir string, stack []string, added map[string]LockEntry) (*Installed, error) {
	var srcDir, commit, lockSource string

	switch ps.kind {
	case specLocal:
		p := expandHome(ps.path)
		if !filepath.IsAbs(p) {
			base := baseDir
			if base == "" {
				var err error
				base, err = os.Getwd()
				if err != nil {
					return nil, fmt.Errorf("install: cannot determine working directory: %w", err)
				}
			}
			p = filepath.Join(base, p)
		}
		p = filepath.Clean(p)
		fi, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("install: local path %s: %w", ps.raw, err)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("install: local path %s is not a directory", ps.raw)
		}
		srcDir = p
		lockSource = p // absolute path: reproducible on this machine

	case specGitHub:
		if _, err := exec.LookPath("git"); err != nil {
			return nil, fmt.Errorf("install: git is required to fetch %s but was not found in PATH: %w", ps.githubURL(), err)
		}
		tmp, err := os.MkdirTemp("", "nvs-pkg-*")
		if err != nil {
			return nil, fmt.Errorf("install: cannot create temp dir: %w", err)
		}
		defer os.RemoveAll(tmp)
		cloneDir := filepath.Join(tmp, "src")
		if err := gitClone(ps.githubURL(), ps.tag, cloneDir); err != nil {
			return nil, err
		}
		commit, err = gitRevParse(cloneDir)
		if err != nil {
			return nil, err
		}
		srcDir = cloneDir
		lockSource = ps.owner + "/" + ps.repo // commit pins it; tag recorded via version check below
	}

	// The manifest is the contract: it carries the real name/version.
	m, err := Load(srcDir)
	if err != nil {
		return nil, fmt.Errorf("install %s: %w", ps.raw, err)
	}

	// Cache key: manifest version for local paths; version + short commit for
	// github sources. Including the commit makes tagged, untagged, and
	// from-lock installs of the same tree land in the same cache directory,
	// which is what keeps InstallFromLock reproducible.
	key := m.Version
	if ps.kind == specGitHub {
		if ps.tag != "" && normalizeTag(ps.tag) != m.Version {
			return nil, fmt.Errorf("install %s: tag %q does not match manifest version %q", ps.raw, ps.tag, m.Version)
		}
		key = m.Version + "-" + shortCommit(commit)
	}

	for _, s := range stack {
		if s == m.Name {
			chain := append(append([]string{}, stack...), m.Name)
			return nil, fmt.Errorf("install: dependency cycle detected: %s", strings.Join(chain, " -> "))
		}
	}

	return installTree(srcDir, m, key, lockSource, commit, ps.raw, workDir, stack, added)
}

// installTree copies srcDir into the cache under key, records the lock entry,
// and installs transitive dependencies.
func installTree(srcDir string, m *Manifest, key, lockSource, commit, rawSpec, workDir string, stack []string, added map[string]LockEntry) (*Installed, error) {
	cacheDir := filepath.Join(cacheRoot(), m.Name+"@"+key)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
			return nil, fmt.Errorf("install: cannot create cache dir: %w", err)
		}
		if err := copyDir(srcDir, cacheDir); err != nil {
			os.RemoveAll(cacheDir)
			return nil, fmt.Errorf("install %s: cannot populate cache: %w", rawSpec, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("install: cannot stat cache dir %s: %w", cacheDir, err)
	}

	meta := cacheMeta{
		Name: m.Name, Version: m.Version, Source: lockSource,
		Commit: commit, InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if data, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(cacheDir, ".nvs-meta.json"), append(data, '\n'), 0o644)
	}

	added[m.Name] = LockEntry{Version: m.Version, Source: lockSource, Commit: commit}

	// Transitive dependencies: relative local dep specs resolve against this
	// package's own directory (cacheDir mirrors the source tree).
	newStack := append(append([]string{}, stack...), m.Name)
	for depName, depSpecRaw := range m.Deps {
		depSpec, err := parseSpec(depSpecRaw)
		if err != nil {
			return nil, fmt.Errorf("install %s: bad dep spec for %q: %w", m.Name, depName, err)
		}
		depInst, err := installOne(depSpec, cacheDir, workDir, newStack, added)
		if err != nil {
			return nil, err
		}
		if depInst.Name != depName {
			return nil, fmt.Errorf("install %s: dep %q resolved to package named %q (manifest name mismatch)", m.Name, depName, depInst.Name)
		}
	}

	main := m.Main
	if main == "" {
		main = "main.nvs"
	}
	mainPath := filepath.Join(cacheDir, main)
	if _, err := os.Stat(mainPath); err != nil {
		return nil, fmt.Errorf("install %s: manifest main %q not found in package", rawSpec, main)
	}
	return &Installed{Name: m.Name, Version: m.Version, Commit: commit, Dir: cacheDir, Main: mainPath}, nil
}

// InstallFromLock installs every package pinned in dir/nvs.lock into the
// cache, honoring the recorded commits. It is the reproducible counterpart
// of Install.
func InstallFromLock(dir string) error {
	lf, err := readLock(dir)
	if err != nil {
		return err
	}
	if len(lf.Packages) == 0 {
		return fmt.Errorf("install: %s not found in %s (nothing to install)", LockFileName, dir)
	}
	added := map[string]LockEntry{}
	for name, e := range lf.Packages {
		if err := installFromLockEntry(name, e, dir, added); err != nil {
			return err
		}
	}
	return nil
}

// InstallFromLock installs from a lockfile, checking out pinned commits for
// github entries.
func installFromLockEntry(name string, e LockEntry, dir string, added map[string]LockEntry) error {
	if e.Commit == "" {
		ps, err := parseSpec(e.Source)
		if err != nil {
			return fmt.Errorf("package %q: %w", name, err)
		}
		inst, err := installOne(ps, "", dir, nil, added)
		if err != nil {
			return err
		}
		if inst.Name != name {
			return fmt.Errorf("expected package %q, got %q", name, inst.Name)
		}
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("install: git is required but was not found in PATH: %w", err)
	}
	parts := strings.SplitN(e.Source, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("package %q: bad github source %q in lockfile", name, e.Source)
	}
	url := "https://github.com/" + parts[0] + "/" + parts[1]
	tmp, err := os.MkdirTemp("", "nvs-pkg-*")
	if err != nil {
		return fmt.Errorf("install: cannot create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)
	cloneDir := filepath.Join(tmp, "src")
	if out, err := gitRun("", "clone", url, cloneDir); err != nil {
		return fmt.Errorf("install: git clone %s failed: %s", url, out)
	}
	if out, err := gitRun(cloneDir, "checkout", e.Commit); err != nil {
		return fmt.Errorf("install: git checkout %s failed: %s", e.Commit, out)
	}
	// Install the checked-out tree into the same cache key an untagged/tagged
	// install would use, so from-lock installs share cache dirs.
	m, err := Load(cloneDir)
	if err != nil {
		return fmt.Errorf("install from lock: package %q: %w", name, err)
	}
	if m.Name != name {
		return fmt.Errorf("install from lock: expected package %q, got %q", name, m.Name)
	}
	key := m.Version + "-" + shortCommit(e.Commit)
	_, err = installTree(cloneDir, m, key, e.Source, e.Commit, e.Source, dir, nil, added)
	if err != nil {
		return err
	}
	return nil
}

func normalizeTag(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

func shortCommit(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	if commit == "" {
		return "unknown"
	}
	return commit
}

// --- git helpers (os/exec, stdlib only) ---

func gitRun(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Never prompt for credentials; fail loudly instead.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func gitClone(url, tag, dest string) error {
	args := []string{"clone", "--depth", "1"}
	if tag != "" {
		args = append(args, "--branch", tag)
	}
	args = append(args, url, dest)
	if out, err := gitRun("", args...); err != nil {
		return fmt.Errorf("install: git clone %s failed: %s", url, out)
	}
	return nil
}

func gitRevParse(dir string) (string, error) {
	out, err := gitRun(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("install: git rev-parse HEAD failed: %s", out)
	}
	return out, nil
}

// --- local helpers ---

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// copyDir copies the directory tree src into dst, skipping .git metadata.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		// Skip version-control metadata; it is not part of the package.
		if info.IsDir() && (info.Name() == ".git" || info.Name() == ".nvs") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
