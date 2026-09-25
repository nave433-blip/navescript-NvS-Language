package polyglot

import (
	"regexp"
	"strings"
)

// DetectLanguage returns language id and a confidence label (high/medium/low).
func DetectLanguage(code string) (id string, confidence string) {
	c := strings.TrimSpace(code)
	if c == "" {
		return "nvs", "low"
	}

	type rule struct {
		id    string
		re    *regexp.Regexp
		score int
	}
	rules := []rule{
		{"python", regexp.MustCompile(`(?m)^\s*def\s+\w+\s*\(|print\s*\(|import\s+\w+|elif\s+`), 3},
		{"js", regexp.MustCompile(`\b(console\.log|const\s+\w+\s*=|let\s+\w+\s*=|function\s*\(|=>\s*\{|require\s*\()`), 3},
		{"ruby", regexp.MustCompile(`(?m)^\s*def\s+\w+|puts\s+|end\s*$|\.each\s+do`), 3},
		{"rust", regexp.MustCompile(`\b(fn\s+main|println!\s*\(|let\s+mut\s+|impl\s+|cargo)`), 4},
		{"go", regexp.MustCompile(`\b(package\s+main|fmt\.Print|func\s+main\s*\(|:=\s*)`), 4},
		{"java", regexp.MustCompile(`\b(public\s+class|System\.out\.print|void\s+main\s*\(|import\s+java\.)`), 4},
		{"cpp", regexp.MustCompile(`\b(std::|#include\s*<iostream>|cout\s*<<|template\s*<)`), 4},
		{"c", regexp.MustCompile(`\b(#include\s*<stdio\.h>|printf\s*\(|int\s+main\s*\()`), 3},
		{"css", regexp.MustCompile(`(?m)^\s*[\w.#-]+\s*\{[^}]*:[^}]*\}`), 3},
		{"nvs", regexp.MustCompile(`(?m)^\s*(let|fn|print|class|match|for\s*\()\b`), 2},
	}

	bestID := "nvs"
	best := 0
	for _, r := range rules {
		if r.re.MatchString(c) {
			if r.score > best {
				best = r.score
				bestID = r.id
			}
		}
	}
	conf := "low"
	if best >= 4 {
		conf = "high"
	} else if best >= 3 {
		conf = "medium"
	}
	return bestID, conf
}
