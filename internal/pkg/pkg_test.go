package pkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempCache points the package cache at a fresh temp dir so tests never
// touch the real ~/.nvs.
func useTempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NVS_PKG_CACHE", dir)
	return dir
}

// writePkg scaffolds a minimal package tree: nvs.json plus the given files.
func writePkg(t *testing.T, dir, name, version string, deps map[string]string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := Manifest{Name: name, Version: version, Description: name + " test package", Main: "main.nvs", Deps: deps}
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInit(t *testing.T) {
	useTempCache(t)
	dir := t.TempDir()

	m, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.Name == "" || m.Version != "0.1.0" || m.Main != "main.nvs" {
		t.Fatalf("Init scaffolded bad manifest: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(dir, "nvs.json")); err != nil {
		t.Fatalf("nvs.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.nvs")); err != nil {
		t.Fatalf("main.nvs stub not written: %v", err)
	}

	// Second init must fail loudly, not overwrite.
	if _, err := Init(dir); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second Init should fail loudly, got: %v", err)
	}
}

func TestLoadValidation(t *testing.T) {
	useTempCache(t)
	cases := []struct {
		name    string
		raw     string
		wantFld string
	}{
		{"missing name", `{"version":"1.0.0"}`, `"name"`},
		{"bad name", `{"name":"Bad Name!","version":"1.0.0"}`, `"name"`},
		{"missing version", `{"name":"ok"}`, `"version"`},
		{"bad version", `{"name":"ok","version":"abc"}`, `"version"`},
		{"bad main", `{"name":"ok","version":"1.0.0","main":"main.txt"}`, `"main"`},
		{"bad dep spec", `{"name":"ok","version":"1.0.0","deps":{"x":"not a spec!!"}}`, `deps["x"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(tc.raw), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("Load should have failed for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantFld) {
				t.Fatalf("error should name field %s, got: %v", tc.wantFld, err)
			}
		})
	}
}

func TestLoadDefaultsMain(t *testing.T) {
	useTempCache(t)
	dir := t.TempDir()
	writePkg(t, dir, "plain", "0.2.0", nil, map[string]string{"main.nvs": "print(1)\n"})
	// Strip the main field; Load must default it.
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = m
	raw := `{"name":"plain","version":"0.2.0"}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	m2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Main != "main.nvs" {
		t.Fatalf("expected default main main.nvs, got %q", m2.Main)
	}
}

func TestInstallLocalAndResolve(t *testing.T) {
	cache := useTempCache(t)
	work := t.TempDir()
	src := filepath.Join(t.TempDir(), "greet")
	writePkg(t, src, "greet", "1.0.0", nil, map[string]string{
		"main.nvs": "print(\"hi\")\n",
		"util.nvs": "fn shout(s) { return s }\n",
	})

	inst, err := Install(src, InstallOptions{WorkDir: work})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if inst.Name != "greet" || inst.Version != "1.0.0" {
		t.Fatalf("bad install result: %+v", inst)
	}
	if !strings.HasPrefix(inst.Dir, cache) {
		t.Fatalf("install dir %q not under temp cache %q", inst.Dir, cache)
	}

	// Resolve from the work dir so the lockfile pins the version.
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	p, ok := Resolve("greet")
	if !ok {
		t.Fatalf("Resolve(greet) failed")
	}
	if !strings.HasSuffix(p, filepath.Join("greet@1.0.0", "main.nvs")) {
		t.Fatalf("Resolve(greet) = %q, want .../greet@1.0.0/main.nvs", p)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("resolved main does not exist: %v", err)
	}

	p2, ok := Resolve("greet/util.nvs")
	if !ok {
		t.Fatalf("Resolve(greet/util.nvs) failed")
	}
	if !strings.HasSuffix(p2, filepath.Join("greet@1.0.0", "util.nvs")) {
		t.Fatalf("Resolve(greet/util.nvs) = %q", p2)
	}

	if _, ok := Resolve("nope"); ok {
		t.Fatalf("Resolve(nope) should return ok=false")
	}
	if _, ok := Resolve("greet/missing.nvs"); ok {
		t.Fatalf("Resolve(greet/missing.nvs) should return ok=false")
	}
}

func TestLockfileWritten(t *testing.T) {
	useTempCache(t)
	work := t.TempDir()
	src := filepath.Join(t.TempDir(), "lib")
	writePkg(t, src, "lib", "2.3.1", nil, map[string]string{"main.nvs": "print(1)\n"})

	if _, err := Install(src, InstallOptions{WorkDir: work}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(work, LockFileName))
	if err != nil {
		t.Fatalf("lockfile not written: %v", err)
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("lockfile invalid JSON: %v", err)
	}
	e, ok := lf.Packages["lib"]
	if !ok {
		t.Fatalf("lockfile missing lib entry: %s", data)
	}
	if e.Version != "2.3.1" {
		t.Fatalf("lock entry version = %q, want 2.3.1", e.Version)
	}
	if e.Source != src {
		t.Fatalf("lock entry source = %q, want %q", e.Source, src)
	}
}

func TestTransitiveDeps(t *testing.T) {
	useTempCache(t)
	work := t.TempDir()
	base := t.TempDir()
	dirB := filepath.Join(base, "pkgb")
	dirA := filepath.Join(base, "pkga")
	writePkg(t, dirB, "pkgb", "0.1.0", nil, map[string]string{"main.nvs": "print(\"b\")\n"})
	writePkg(t, dirA, "pkga", "0.1.0", map[string]string{"pkgb": dirB}, map[string]string{"main.nvs": "print(\"a\")\n"})

	if _, err := Install(dirA, InstallOptions{WorkDir: work}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	lf, err := readLock(work)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lf.Packages["pkga"]; !ok {
		t.Fatalf("lockfile missing pkga")
	}
	if _, ok := lf.Packages["pkgb"]; !ok {
		t.Fatalf("lockfile missing transitive dep pkgb")
	}
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	if _, ok := Resolve("pkgb"); !ok {
		t.Fatalf("transitive dep pkgb not resolvable")
	}
}

func TestCycleDetection(t *testing.T) {
	useTempCache(t)
	work := t.TempDir()
	base := t.TempDir()
	dirA := filepath.Join(base, "ca")
	dirB := filepath.Join(base, "cb")
	writePkg(t, dirA, "ca", "0.1.0", map[string]string{"cb": dirB}, map[string]string{"main.nvs": "print(1)\n"})
	writePkg(t, dirB, "cb", "0.1.0", map[string]string{"ca": dirA}, map[string]string{"main.nvs": "print(1)\n"})

	_, err := Install(dirA, InstallOptions{WorkDir: work})
	if err == nil {
		t.Fatalf("Install should fail on dependency cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error should say 'cycle', got: %v", err)
	}
}

func TestInstallFromLock(t *testing.T) {
	cache := useTempCache(t)
	work := t.TempDir()
	src := filepath.Join(t.TempDir(), "relock")
	writePkg(t, src, "relock", "1.0.0", nil, map[string]string{"main.nvs": "print(1)\n"})

	if _, err := Install(src, InstallOptions{WorkDir: work}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Wipe the cache entirely, then reproduce from the lockfile.
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	if err := InstallFromLock(work); err != nil {
		t.Fatalf("InstallFromLock: %v", err)
	}
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	if _, ok := Resolve("relock"); !ok {
		t.Fatalf("Resolve(relock) failed after InstallFromLock")
	}
}

func TestRemove(t *testing.T) {
	useTempCache(t)
	work := t.TempDir()
	src := filepath.Join(t.TempDir(), "gone")
	writePkg(t, src, "gone", "1.0.0", nil, map[string]string{"main.nvs": "print(1)\n"})

	inst, err := Install(src, InstallOptions{WorkDir: work})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	if err := Remove("gone"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(inst.Dir); !os.IsNotExist(err) {
		t.Fatalf("cache dir still exists after Remove")
	}
	if _, ok := Resolve("gone"); ok {
		t.Fatalf("Resolve(gone) should fail after Remove")
	}
	lf, err := readLock(work)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lf.Packages["gone"]; ok {
		t.Fatalf("lockfile still lists gone after Remove")
	}
	if err := Remove("gone"); err == nil {
		t.Fatalf("second Remove should fail loudly")
	}
}

func TestList(t *testing.T) {
	useTempCache(t)
	work := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		src := filepath.Join(t.TempDir(), name)
		writePkg(t, src, name, "1.0.0", nil, map[string]string{"main.nvs": "print(1)\n"})
		if _, err := Install(src, InstallOptions{WorkDir: work}); err != nil {
			t.Fatalf("Install %s: %v", name, err)
		}
	}
	got, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("List = %+v, want alpha+beta", got)
	}
	if got[0].Source == "" {
		t.Fatalf("List entry missing source metadata")
	}
}

func TestPublishCheck(t *testing.T) {
	useTempCache(t)
	dir := t.TempDir()
	writePkg(t, dir, "shipit", "1.2.0", nil, map[string]string{"main.nvs": "print(1)\n"})

	steps, err := PublishCheck(dir)
	if err != nil {
		t.Fatalf("PublishCheck: %v", err)
	}
	for _, want := range []string{"git tag", "1.2.0", "no central", "GitHub"} {
		if !strings.Contains(steps, want) {
			t.Fatalf("publish steps should mention %q:\n%s", want, steps)
		}
	}

	// Empty description must fail.
	raw := `{"name":"shipit","version":"1.2.0","main":"main.nvs"}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishCheck(dir); err == nil || !strings.Contains(err.Error(), `"description"`) {
		t.Fatalf("PublishCheck should demand a description, got: %v", err)
	}
}

func TestBadSpecs(t *testing.T) {
	useTempCache(t)
	work := t.TempDir()
	if _, err := Install("", InstallOptions{WorkDir: work}); err == nil {
		t.Fatalf("empty spec should fail")
	}
	if _, err := Install("justaword", InstallOptions{WorkDir: work}); err == nil {
		t.Fatalf("bare word spec should fail")
	}
	if _, err := Install("./does-not-exist", InstallOptions{WorkDir: work}); err == nil {
		t.Fatalf("missing local path should fail loudly")
	}
}
