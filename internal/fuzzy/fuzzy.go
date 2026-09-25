// Package fuzzy implements simple fuzzy string matching and ranking.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"
)

// Score returns a similarity score in [0, 1] between query and candidate.
// Uses subsequence matching with bonuses for contiguous and boundary matches.
func Score(query, candidate string) float64 {
	q := []rune(strings.ToLower(query))
	c := []rune(strings.ToLower(candidate))
	if len(q) == 0 {
		return 1
	}
	if len(c) == 0 {
		return 0
	}
	// Exact
	if string(q) == string(c) {
		return 1
	}
	// Substring
	if strings.Contains(string(c), string(q)) {
		return 0.9 + 0.1*(float64(len(q))/float64(len(c)))
	}

	qi := 0
	matches := 0
	cont := 0
	bestCont := 0
	boundary := 0
	for ci := 0; ci < len(c) && qi < len(q); ci++ {
		if c[ci] == q[qi] {
			matches++
			cont++
			if cont > bestCont {
				bestCont = cont
			}
			// word boundary bonus
			if ci == 0 || isBoundary(c[ci-1]) {
				boundary++
			}
			qi++
		} else {
			cont = 0
		}
	}
	if matches != len(q) {
		// partial subsequence
		if matches == 0 {
			// fallback: levenshtein-ish ratio
			return 1.0 - float64(levenshtein(string(q), string(c)))/float64(max(len(q), len(c)))
		}
		return float64(matches) / float64(len(q)) * 0.5
	}
	// full subsequence matched
	score := 0.55
	score += 0.2 * (float64(bestCont) / float64(len(q)))
	score += 0.15 * (float64(boundary) / float64(len(q)))
	score += 0.1 * (float64(len(q)) / float64(len(c)))
	if score > 1 {
		score = 1
	}
	return score
}

func isBoundary(r rune) bool {
	return unicode.IsSpace(r) || r == '_' || r == '-' || r == '/' || r == '.'
}

// Match is true if score >= threshold (default 0.3 if threshold <= 0).
func Match(query, candidate string, threshold float64) bool {
	if threshold <= 0 {
		threshold = 0.3
	}
	return Score(query, candidate) >= threshold
}

// Ranked is a candidate with its score.
type Ranked struct {
	Value string
	Score float64
}

// Find returns candidates ranked by fuzzy score, filtered by threshold.
func Find(query string, candidates []string, threshold float64, limit int) []Ranked {
	if threshold <= 0 {
		threshold = 0.3
	}
	var out []Ranked
	for _, c := range candidates {
		s := Score(query, c)
		if s >= threshold {
			out = append(out, Ranked{Value: c, Score: s})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Value < out[j].Value
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Best returns the best matching candidate or "".
func Best(query string, candidates []string, threshold float64) string {
	r := Find(query, candidates, threshold, 1)
	if len(r) == 0 {
		return ""
	}
	return r[0].Value
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = min(del, min(ins, sub))
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
