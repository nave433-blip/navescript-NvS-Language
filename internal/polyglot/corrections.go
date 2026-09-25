package polyglot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Correction is a learned fix for a translation pair.
type Correction struct {
	ID         string `json:"id"`
	FromLang   string `json:"from_lang"`
	ToLang     string `json:"to_lang"`
	Source     string `json:"source"`
	SourceHash string `json:"source_hash"`
	BadOutput  string `json:"bad_output,omitempty"`
	Corrected  string `json:"corrected"`
	Note       string `json:"note,omitempty"`
	Hits       int    `json:"hits"`
	Created    string `json:"created"`
	Updated    string `json:"updated,omitempty"`
}

type correctionDB struct {
	Corrections []Correction `json:"corrections"`
}

var (
	corrMu   sync.RWMutex
	corrPath string
	corrMem  = correctionDB{}
	corrLoad sync.Once
)

func defaultCorrPath() string {
	if p := os.Getenv("NVS_CORRECTIONS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "nvs_corrections.json"
	}
	return filepath.Join(home, ".nvs", "corrections.json")
}

// SetCorrectionsPath sets the JSON DB path and reloads.
func SetCorrectionsPath(path string) {
	corrMu.Lock()
	defer corrMu.Unlock()
	corrPath = path
	_ = loadCorrectionsLocked()
}

// CorrectionsPath returns the active DB path.
func CorrectionsPath() string {
	corrMu.RLock()
	defer corrMu.RUnlock()
	if corrPath == "" {
		return defaultCorrPath()
	}
	return corrPath
}

func ensureCorrLoaded() {
	corrLoad.Do(func() {
		corrMu.Lock()
		defer corrMu.Unlock()
		if corrPath == "" {
			corrPath = defaultCorrPath()
		}
		_ = loadCorrectionsLocked()
	})
}

func loadCorrectionsLocked() error {
	path := corrPath
	if path == "" {
		path = defaultCorrPath()
		corrPath = path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			corrMem = correctionDB{}
			return nil
		}
		return err
	}
	var db correctionDB
	if err := json.Unmarshal(data, &db); err != nil {
		return err
	}
	corrMem = db
	return nil
}

func saveCorrectionsLocked() error {
	path := corrPath
	if path == "" {
		path = defaultCorrPath()
		corrPath = path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		// if dir is "." MkdirAll is fine; if path has no dir, Dir is "."
		_ = err
	}
	data, err := json.MarshalIndent(corrMem, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func hashSource(from, to, source string) string {
	h := sha256.Sum256([]byte(strings.ToLower(from) + "|" + strings.ToLower(to) + "|" + source))
	return hex.EncodeToString(h[:8])
}

// AddCorrection stores a correction and persists the DB.
func AddCorrection(from, to, source, corrected, bad, note string) (Correction, error) {
	ensureCorrLoaded()
	corrMu.Lock()
	defer corrMu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	c := Correction{
		ID:         fmt.Sprintf("corr_%d", time.Now().UnixNano()),
		FromLang:   strings.ToLower(strings.TrimSpace(from)),
		ToLang:     strings.ToLower(strings.TrimSpace(to)),
		Source:     source,
		SourceHash: hashSource(from, to, source),
		BadOutput:  bad,
		Corrected:  corrected,
		Note:       note,
		Hits:       0,
		Created:    now,
	}
	// replace existing same hash
	found := false
	for i := range corrMem.Corrections {
		if corrMem.Corrections[i].SourceHash == c.SourceHash {
			c.ID = corrMem.Corrections[i].ID
			c.Hits = corrMem.Corrections[i].Hits
			c.Created = corrMem.Corrections[i].Created
			c.Updated = now
			corrMem.Corrections[i] = c
			found = true
			break
		}
	}
	if !found {
		corrMem.Corrections = append(corrMem.Corrections, c)
	}
	if err := saveCorrectionsLocked(); err != nil {
		return c, err
	}
	return c, nil
}

// PullCorrection finds a correction for from/to/source (exact hash, then contains).
// Increments hit count when found.
func PullCorrection(from, to, source string) (Correction, bool) {
	ensureCorrLoaded()
	corrMu.Lock()
	defer corrMu.Unlock()
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	h := hashSource(from, to, source)

	// exact hash
	for i := range corrMem.Corrections {
		c := &corrMem.Corrections[i]
		if c.SourceHash == h {
			c.Hits++
			_ = saveCorrectionsLocked()
			return *c, true
		}
	}
	// substring match on source (longest wins)
	var best *Correction
	bestLen := 0
	for i := range corrMem.Corrections {
		c := &corrMem.Corrections[i]
		if c.FromLang != from || c.ToLang != to {
			continue
		}
		if c.Source != "" && strings.Contains(source, c.Source) {
			if len(c.Source) > bestLen {
				best = c
				bestLen = len(c.Source)
			}
		}
	}
	if best != nil {
		best.Hits++
		_ = saveCorrectionsLocked()
		return *best, true
	}
	return Correction{}, false
}

// ListCorrections returns a copy of all corrections, optionally filtered by langs.
func ListCorrections(from, to string) []Correction {
	ensureCorrLoaded()
	corrMu.RLock()
	defer corrMu.RUnlock()
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	var out []Correction
	for _, c := range corrMem.Corrections {
		if from != "" && c.FromLang != from {
			continue
		}
		if to != "" && c.ToLang != to {
			continue
		}
		out = append(out, c)
	}
	return out
}

// ApplyCorrections runs translation then overlays any learned correction.
func ApplyCorrections(from, to, source, translated string) string {
	if c, ok := PullCorrection(from, to, source); ok && c.Corrected != "" {
		return c.Corrected
	}
	return translated
}

// CorrectionsJSON dumps the DB for inspection.
func CorrectionsJSON(from, to string) string {
	list := ListCorrections(from, to)
	b, _ := json.MarshalIndent(list, "", "  ")
	return string(b)
}
