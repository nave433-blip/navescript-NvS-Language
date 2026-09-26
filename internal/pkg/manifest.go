package pkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Manifest is the parsed contents of nvs.json.
type Manifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Wave 17: description and deps are always serialized (never
	// omitted), so `nvs pkg init` scaffolds them explicitly and every
	// manifest states its dependencies up front — even when empty.
	Description string            `json:"description"`
	Main        string            `json:"main,omitempty"`
	Deps        map[string]string `json:"deps"`
}

var (
	nameRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
)

// Init scaffolds a new nvs.json in dir (plus a minimal main.nvs when the
// manifest's main file does not exist yet). It fails loudly if dir already
// contains an nvs.json.
func Init(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("init: %s already exists in %s; refusing to overwrite", ManifestFileName, dir)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("init: cannot stat %s: %w", path, err)
	}
	base := filepath.Base(dir)
	if base == "." || base == "/" || base == string(filepath.Separator) {
		base = "mypackage"
	}
	name := sanitizeName(base)
	m := &Manifest{
		Name:        name,
		Version:     "0.1.0",
		Description: "",
		Main:        "main.nvs",
		Deps:        map[string]string{},
	}
	if err := writeManifest(path, m); err != nil {
		return nil, err
	}
	mainPath := filepath.Join(dir, m.Main)
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		stub := fmt.Sprintf("// %s — package entry point\n\nfn main() {\n\tprint(\"hello from %s\")\n}\n", name, name)
		if err := os.WriteFile(mainPath, []byte(stub), 0o644); err != nil {
			return nil, fmt.Errorf("init: cannot write %s: %w", m.Main, err)
		}
	}
	return m, nil
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ' || r == '+':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if out == "" {
		out = "mypackage"
	}
	return out
}

// Load reads and validates the nvs.json in dir. Every validation error
// names the offending field.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load: cannot read %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("load: %s is not valid JSON: %w", ManifestFileName, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if m.Main == "" {
		m.Main = "main.nvs"
	}
	return &m, nil
}

// Validate reports the first invalid field, naming it.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf(`invalid manifest: field "name" is required`)
	}
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf(`invalid manifest: field "name" %q must match [a-z0-9][a-z0-9._-]*`, m.Name)
	}
	if m.Version == "" {
		return fmt.Errorf(`invalid manifest: field "version" is required`)
	}
	if !versionRe.MatchString(m.Version) {
		return fmt.Errorf(`invalid manifest: field "version" %q must be semver (e.g. "1.2.3")`, m.Version)
	}
	if m.Main != "" && !strings.HasSuffix(m.Main, ".nvs") && !strings.HasSuffix(m.Main, ".ns") {
		return fmt.Errorf(`invalid manifest: field "main" %q must end in .nvs or .ns`, m.Main)
	}
	for depName, depSpec := range m.Deps {
		if !nameRe.MatchString(depName) {
			return fmt.Errorf(`invalid manifest: deps key %q is not a valid package name`, depName)
		}
		if _, err := parseSpec(depSpec); err != nil {
			return fmt.Errorf("invalid manifest: deps[%q] has bad spec: %w", depName, err)
		}
	}
	return nil
}

func writeManifest(path string, m *Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}
