package pkg

import (
	"fmt"
	"strings"
)

// specKind classifies an install spec.
type specKind int

const (
	specGitHub specKind = iota // user/repo or user/repo@tag
	specLocal                  // ./rel, ../rel, /abs, ~/home
)

// parsedSpec is a validated install spec.
type parsedSpec struct {
	kind  specKind
	raw   string
	owner string // github only
	repo  string // github only
	tag   string // github only, "" = default branch
	path  string // local only, as given
}

func (s parsedSpec) githubURL() string {
	return "https://github.com/" + s.owner + "/" + s.repo
}

// parseSpec accepts:
//   - user/repo        (GitHub, default branch)
//   - user/repo@tag     (GitHub, at tag/branch)
//   - ./mylib, ../lib, /abs/path, ~/path  (local directory)
func parseSpec(raw string) (parsedSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return parsedSpec{}, fmt.Errorf("empty package spec")
	}
	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") ||
		raw == "." || raw == ".." ||
		strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "~/") || raw == "~" {
		return parsedSpec{kind: specLocal, raw: raw, path: raw}, nil
	}
	// GitHub form: user/repo[@tag]. The @tag separator is the LAST @ so
	// repo names containing @ still parse (they can't, but be safe).
	spec := raw
	tag := ""
	if i := strings.LastIndex(spec, "@"); i >= 0 {
		tag = spec[i+1:]
		spec = spec[:i]
		if tag == "" {
			return parsedSpec{}, fmt.Errorf("bad spec %q: empty tag after @", raw)
		}
	}
	parts := strings.Split(spec, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return parsedSpec{}, fmt.Errorf("bad spec %q: want user/repo[@tag] or a local path (./x, /abs/x)", raw)
	}
	return parsedSpec{kind: specGitHub, raw: raw, owner: parts[0], repo: parts[1], tag: tag}, nil
}
