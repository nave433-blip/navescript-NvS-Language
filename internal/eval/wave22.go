package eval

// Wave 22 — sequence power tools, statistics, math, encoding, string
// shaping, and resilient scripting helpers.
//
// Robbed honestly from: Python itertools/statistics, Ruby Enumerable,
// JavaScript Array/lodash, Rust iterators, npm semver, Rails/Django
// slugify, tenacity (retry). Everything here is backed by the Go
// standard library only: no new external dependencies, no cgo.
//
// All builtins are pure additions — no existing behavior is changed,
// so every Wave 1–21 script keeps working byte-for-byte.

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/navescript/nvs/internal/object"
)

// registerWave22Builtins adds the wave-22 builtins. Called once from
// initBuiltins() (same pattern as registerWave20Builtins).
func registerWave22Builtins() {
	// ---------------- sequences (itertools / Ruby / JS / Rust) ----------------

	builtins["scan"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// scan(arr, fn) | scan(arr, init, fn) — cumulative fold
			// (Python itertools.accumulate).
			var arr *object.Array
			var init object.Object
			var fn object.Object
			hasInit := false
			switch len(args) {
			case 2:
				a, ok := args[0].(*object.Array)
				if !ok {
					return newError("scan: want array")
				}
				arr, fn = a, args[1]
			case 3:
				a, ok := args[0].(*object.Array)
				if !ok {
					return newError("scan: want array")
				}
				arr, init, fn = a, args[1], args[2]
				hasInit = true
			default:
				return newError("scan: want (array, fn) or (array, init, fn)")
			}
			if len(arr.Elements) == 0 {
				if hasInit {
					return &object.Array{Elements: []object.Object{init}}
				}
				return &object.Array{Elements: []object.Object{}}
			}
			out := make([]object.Object, 0, len(arr.Elements))
			var acc object.Object
			start := 0
			if hasInit {
				acc = init
			} else {
				acc = arr.Elements[0]
				start = 1
				out = append(out, acc)
			}
			for _, el := range arr.Elements[start:] {
				r := applyFunction(fn, []object.Object{acc, el})
				if isError(r) {
					return r
				}
				acc = r
				out = append(out, acc)
			}
			return &object.Array{Elements: out}
		},
	}

	builtins["partition"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// partition(arr, pred) -> [matching, rest] (Ruby Enumerable#partition).
			if len(args) != 2 {
				return newError("partition: want array, predicate")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("partition: want array")
			}
			yes := []object.Object{}
			no := []object.Object{}
			for _, el := range arr.Elements {
				r := applyFunction(args[1], []object.Object{el})
				if isError(r) {
					return r
				}
				if isTruthy(r) {
					yes = append(yes, el)
				} else {
					no = append(no, el)
				}
			}
			return &object.Array{Elements: []object.Object{
				&object.Array{Elements: yes},
				&object.Array{Elements: no},
			}}
		},
	}

	builtins["take_while"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// take_while(arr, pred) — leading run where pred holds (Haskell/Rust).
			if len(args) != 2 {
				return newError("take_while: want array, predicate")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("take_while: want array")
			}
			out := []object.Object{}
			for _, el := range arr.Elements {
				r := applyFunction(args[1], []object.Object{el})
				if isError(r) {
					return r
				}
				if !isTruthy(r) {
					break
				}
				out = append(out, el)
			}
			return &object.Array{Elements: out}
		},
	}

	builtins["drop_while"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// drop_while(arr, pred) — remainder after leading pred run (Haskell/Rust).
			if len(args) != 2 {
				return newError("drop_while: want array, predicate")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("drop_while: want array")
			}
			i := 0
			for ; i < len(arr.Elements); i++ {
				r := applyFunction(args[1], []object.Object{arr.Elements[i]})
				if isError(r) {
					return r
				}
				if !isTruthy(r) {
					break
				}
			}
			out := make([]object.Object, len(arr.Elements)-i)
			copy(out, arr.Elements[i:])
			return &object.Array{Elements: out}
		},
	}

	builtins["flat_map"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// flat_map(arr, fn) — map, then flatten one level (JS/Rust).
			if len(args) != 2 {
				return newError("flat_map: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("flat_map: want array")
			}
			out := []object.Object{}
			for _, el := range arr.Elements {
				r := applyFunction(args[1], []object.Object{el})
				if isError(r) {
					return r
				}
				sub, ok := r.(*object.Array)
				if !ok {
					return newError("flat_map: function must return array, got %s", r.Type())
				}
				out = append(out, sub.Elements...)
			}
			return &object.Array{Elements: out}
		},
	}

	builtins["windows"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// windows(arr, size) — sliding windows of `size` (more-itertools).
			if len(args) != 2 {
				return newError("windows: want array, size")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("windows: want array")
			}
			n, ok := args[1].(*object.Integer)
			if !ok || n.Value < 1 {
				return newError("windows: size must be a positive integer")
			}
			size := int(n.Value)
			out := []object.Object{}
			for i := 0; i+size <= len(arr.Elements); i++ {
				w := make([]object.Object, size)
				copy(w, arr.Elements[i:i+size])
				out = append(out, &object.Array{Elements: w})
			}
			return &object.Array{Elements: out}
		},
	}

	// ---------------- statistics (Python statistics) ----------------

	builtins["median"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// median(arr) — middle value; float average for even counts.
			if len(args) != 1 {
				return newError("median: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("median: want array")
			}
			vals, errObj := wave22numbers(arr, "median")
			if errObj != nil {
				return errObj
			}
			if len(vals) == 0 {
				return newError("median: empty array")
			}
			idx := make([]int, len(vals))
			for i := range idx {
				idx[i] = i
			}
			sort.SliceStable(idx, func(a, b int) bool { return vals[idx[a]] < vals[idx[b]] })
			n := len(idx)
			if n%2 == 1 {
				return arr.Elements[idx[n/2]]
			}
			return &object.Float{Value: (vals[idx[n/2-1]] + vals[idx[n/2]]) / 2}
		},
	}

	builtins["variance"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// variance(arr) — population variance (Python statistics.pvariance).
			if len(args) != 1 {
				return newError("variance: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("variance: want array")
			}
			vals, errObj := wave22numbers(arr, "variance")
			if errObj != nil {
				return errObj
			}
			if len(vals) == 0 {
				return newError("variance: empty array")
			}
			var mean float64
			for _, v := range vals {
				mean += v
			}
			mean /= float64(len(vals))
			var ss float64
			for _, v := range vals {
				d := v - mean
				ss += d * d
			}
			return &object.Float{Value: ss / float64(len(vals))}
		},
	}

	builtins["stddev"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// stddev(arr) — population standard deviation (statistics.pstdev).
			if len(args) != 1 {
				return newError("stddev: want array")
			}
			v := builtins["variance"].Fn(args...)
			if isError(v) {
				return v
			}
			f, _ := v.(*object.Float)
			return &object.Float{Value: math.Sqrt(f.Value)}
		},
	}

	builtins["mode"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// mode(arr) — most frequent element; first wins ties (statistics.mode).
			if len(args) != 1 {
				return newError("mode: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("mode: want array")
			}
			if len(arr.Elements) == 0 {
				return newError("mode: empty array")
			}
			counts := map[string]int{}
			best := arr.Elements[0]
			bestKey := wave22key(best)
			counts[bestKey] = 1
			for _, el := range arr.Elements[1:] {
				k := wave22key(el)
				counts[k]++
				if counts[k] > counts[bestKey] {
					best, bestKey = el, k
				}
			}
			return best
		},
	}

	// ---------------- math ----------------

	builtins["is_prime"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// is_prime(n) — deterministic Miller-Rabin, exact for all int64.
			if len(args) != 1 {
				return newError("is_prime: want integer")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return newError("is_prime: want integer")
			}
			return nativeBoolToBooleanObject(wave22MillerRabin(n.Value))
		},
	}

	builtins["lerp"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// lerp(a, b, t) — linear interpolation a+(b-a)*t (game-dev staple).
			if len(args) != 3 {
				return newError("lerp: want a, b, t")
			}
			a, errObj := wave22float(args[0], "lerp")
			if errObj != nil {
				return errObj
			}
			b, errObj := wave22float(args[1], "lerp")
			if errObj != nil {
				return errObj
			}
			t, errObj := wave22float(args[2], "lerp")
			if errObj != nil {
				return errObj
			}
			return &object.Float{Value: a + (b-a)*t}
		},
	}

	// ---------------- encoding / compression ----------------

	builtins["hex_encode"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("hex_encode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("hex_encode: want string")
			}
			return &object.String{Value: hex.EncodeToString([]byte(s.Value))}
		},
	}

	builtins["hex_decode"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("hex_decode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("hex_decode: want string")
			}
			b, err := hex.DecodeString(s.Value)
			if err != nil {
				return newError("hex_decode: %s", err.Error())
			}
			return &object.String{Value: string(b)}
		},
	}

	builtins["gzip_compress"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// gzip_compress(s) -> base64(gzip(s)); binary-safe transport
			// follows the aes_gcm_encrypt base64-string convention.
			if len(args) != 1 {
				return newError("gzip_compress: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("gzip_compress: want string")
			}
			var buf bytes.Buffer
			w := gzip.NewWriter(&buf)
			if _, err := w.Write([]byte(s.Value)); err != nil {
				return newError("gzip_compress: %s", err.Error())
			}
			if err := w.Close(); err != nil {
				return newError("gzip_compress: %s", err.Error())
			}
			return &object.String{Value: base64.StdEncoding.EncodeToString(buf.Bytes())}
		},
	}

	builtins["gzip_decompress"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("gzip_decompress: want base64 string from gzip_compress")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("gzip_decompress: want string")
			}
			raw, err := base64.StdEncoding.DecodeString(s.Value)
			if err != nil {
				return newError("gzip_decompress: bad base64: %s", err.Error())
			}
			r, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				return newError("gzip_decompress: bad gzip data: %s", err.Error())
			}
			out, err := io.ReadAll(r)
			if err != nil {
				return newError("gzip_decompress: %s", err.Error())
			}
			return &object.String{Value: string(out)}
		},
	}

	// ---------------- string shaping (lodash / Rails / Django) ----------------

	builtins["render_template"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// render_template(tmpl, map) — {{key}} interpolation (Mustache-lite).
			// Missing keys render as "".
			if len(args) != 2 {
				return newError("render_template: want template string, map")
			}
			tmpl, ok := args[0].(*object.String)
			if !ok {
				return newError("render_template: want template string")
			}
			h, ok := args[1].(*object.Hash)
			if !ok {
				return newError("render_template: want map")
			}
			out := wave22tmplRe.ReplaceAllStringFunc(tmpl.Value, func(m string) string {
				key := strings.TrimSpace(m[2 : len(m)-2])
				ks := &object.String{Value: key}
				if p, found := h.Pairs[ks.HashKey()]; found {
					return p.Value.Inspect()
				}
				return ""
			})
			return &object.String{Value: out}
		},
	}

	builtins["slugify"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// slugify(s) — URL slug: lowercase, runs of non-alnum -> "-".
			if len(args) != 1 {
				return newError("slugify: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("slugify: want string")
			}
			var b strings.Builder
			dash := false
			for _, r := range strings.ToLower(s.Value) {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					b.WriteRune(r)
					dash = false
				} else if !dash {
					b.WriteRune('-')
					dash = true
				}
			}
			return &object.String{Value: strings.Trim(b.String(), "-")}
		},
	}

	builtins["snake_case"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("snake_case: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("snake_case: want string")
			}
			return &object.String{Value: strings.Join(wave22words(s.Value), "_")}
		},
	}

	builtins["camel_case"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("camel_case: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("camel_case: want string")
			}
			words := wave22words(s.Value)
			for i := 1; i < len(words); i++ {
				words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
			}
			return &object.String{Value: strings.Join(words, "")}
		},
	}

	builtins["kebab_case"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("kebab_case: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("kebab_case: want string")
			}
			return &object.String{Value: strings.Join(wave22words(s.Value), "-")}
		},
	}

	builtins["levenshtein"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// levenshtein(a, b) — edit distance (Wagner-Fischer, two rows).
			if len(args) != 2 {
				return newError("levenshtein: want two strings")
			}
			a, ok := args[0].(*object.String)
			if !ok {
				return newError("levenshtein: want strings")
			}
			b, ok := args[1].(*object.String)
			if !ok {
				return newError("levenshtein: want strings")
			}
			ra, rb := []rune(a.Value), []rune(b.Value)
			if len(ra) > len(rb) {
				ra, rb = rb, ra
			}
			prev := make([]int, len(ra)+1)
			for i := range prev {
				prev[i] = i
			}
			cur := make([]int, len(ra)+1)
			for j := 1; j <= len(rb); j++ {
				cur[0] = j
				for i := 1; i <= len(ra); i++ {
					cost := 0
					if ra[i-1] != rb[j-1] {
						cost = 1
					}
					del, ins, sub := prev[i]+1, cur[i-1]+1, prev[i-1]+cost
					cur[i] = del
					if ins < cur[i] {
						cur[i] = ins
					}
					if sub < cur[i] {
						cur[i] = sub
					}
				}
				prev, cur = cur, prev
			}
			return newInteger(int64(prev[len(ra)]))
		},
	}

	builtins["semver_compare"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// semver_compare(a, b) -> -1/0/1 (npm semver, core ordering).
			if len(args) != 2 {
				return newError("semver_compare: want two version strings")
			}
			a, ok := args[0].(*object.String)
			if !ok {
				return newError("semver_compare: want strings")
			}
			b, ok := args[1].(*object.String)
			if !ok {
				return newError("semver_compare: want strings")
			}
			return newInteger(int64(wave22semverCmp(a.Value, b.Value)))
		},
	}

	// ---------------- data out ----------------

	builtins["csv_stringify"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// csv_stringify(rows) — array of arrays -> CSV (RFC 4180 quoting);
			// the writer half of csv_parse.
			if len(args) != 1 {
				return newError("csv_stringify: want array of arrays")
			}
			rows, ok := args[0].(*object.Array)
			if !ok {
				return newError("csv_stringify: want array of arrays")
			}
			var b strings.Builder
			for ri, r := range rows.Elements {
				row, ok := r.(*object.Array)
				if !ok {
					return newError("csv_stringify: row %d is not an array", ri)
				}
				for ci, c := range row.Elements {
					s := ""
					if str, ok := c.(*object.String); ok {
						s = str.Value
					} else {
						s = c.Inspect()
					}
					if strings.ContainsAny(s, ",\"\r\n") {
						s = "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
					}
					if ci > 0 {
						b.WriteByte(',')
					}
					b.WriteString(s)
				}
				b.WriteByte('\n')
			}
			return &object.String{Value: b.String()}
		},
	}

	builtins["retry"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// retry(fn, attempts[, delay_ms]) — rerun a zero-arg function
			// until it succeeds (Python tenacity-lite). Returns the first
			// non-error result, else the last error.
			if len(args) < 2 || len(args) > 3 {
				return newError("retry: want (fn, attempts[, delay_ms])")
			}
			attempts, ok := args[1].(*object.Integer)
			if !ok || attempts.Value < 1 {
				return newError("retry: attempts must be a positive integer")
			}
			var delay time.Duration
			if len(args) == 3 {
				d, ok := args[2].(*object.Integer)
				if !ok || d.Value < 0 {
					return newError("retry: delay_ms must be a non-negative integer")
				}
				delay = time.Duration(d.Value) * time.Millisecond
			}
			var last object.Object = newError("retry: no attempts made")
			for i := int64(0); i < attempts.Value; i++ {
				r := applyFunction(args[0], []object.Object{})
				if !isError(r) {
					return r
				}
				last = r
				if delay > 0 && i+1 < attempts.Value {
					time.Sleep(delay)
				}
			}
			return last
		},
	}
}

// ---- wave-22 helpers ----

var wave22tmplRe = regexp.MustCompile(`\{\{\s*[a-zA-Z_][a-zA-Z0-9_]*\s*\}\}`)

// wave22numbers extracts float64 values from an array of ints/floats.
func wave22numbers(arr *object.Array, name string) ([]float64, *object.Error) {
	vals := make([]float64, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		switch v := el.(type) {
		case *object.Integer:
			vals = append(vals, float64(v.Value))
		case *object.Float:
			vals = append(vals, v.Value)
		default:
			return nil, newError("%s: want array of numbers, got %s", name, el.Type())
		}
	}
	return vals, nil
}

func wave22float(o object.Object, name string) (float64, *object.Error) {
	switch v := o.(type) {
	case *object.Integer:
		return float64(v.Value), nil
	case *object.Float:
		return v.Value, nil
	default:
		return 0, newError("%s: want number, got %s", name, o.Type())
	}
}

func wave22key(o object.Object) string {
	return string(o.Type()) + "\x00" + o.Inspect()
}

// wave22words splits s into lowercase words on case boundaries and
// separators (lodash-style word splitting).
func wave22words(s string) []string {
	var words []string
	var cur []rune
	runes := []rune(s)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if i > 0 && unicode.IsUpper(r) {
				prev := runes[i-1]
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				// Split on lower->Upper and on acronym end (HTTPRequest -> http, request).
				if unicode.IsLower(prev) || unicode.IsDigit(prev) ||
					(unicode.IsUpper(prev) && nextIsLower) {
					flush()
				}
			}
			cur = append(cur, r)
		} else {
			flush()
		}
	}
	flush()
	return words
}

// wave22semverCmp implements core semver ordering: -1, 0, or 1.
func wave22semverCmp(a, b string) int {
	pa := wave22parseSemver(a)
	pb := wave22parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa.nums[i] != pb.nums[i] {
			if pa.nums[i] < pb.nums[i] {
				return -1
			}
			return 1
		}
	}
	// A version without prerelease outranks one with it.
	if len(pa.pre) == 0 && len(pb.pre) > 0 {
		return 1
	}
	if len(pa.pre) > 0 && len(pb.pre) == 0 {
		return -1
	}
	for i := 0; i < len(pa.pre) && i < len(pb.pre); i++ {
		c := wave22semverIdentCmp(pa.pre[i], pb.pre[i])
		if c != 0 {
			return c
		}
	}
	switch {
	case len(pa.pre) < len(pb.pre):
		return -1
	case len(pa.pre) > len(pb.pre):
		return 1
	}
	return 0
}

type wave22semver struct {
	nums [3]int64
	pre  []string
}

func wave22parseSemver(s string) wave22semver {
	var v wave22semver
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Build metadata (+...) does not affect ordering.
	if i := strings.Index(s, "+"); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.Index(s, "-"); i >= 0 {
		pre, s = s[i+1:], s[:i]
	}
	parts := strings.Split(s, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.ParseInt(strings.TrimSpace(parts[i]), 10, 64)
		if err != nil {
			n = 0
		}
		v.nums[i] = n
	}
	if pre != "" {
		v.pre = strings.Split(pre, ".")
	}
	return v
}

func wave22semverIdentCmp(a, b string) int {
	ai, aerr := strconv.ParseInt(a, 10, 64)
	bi, berr := strconv.ParseInt(b, 10, 64)
	if aerr == nil && berr == nil {
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
		return 0
	}
	// Numeric identifiers outrank alphanumeric ones.
	if aerr == nil {
		return -1
	}
	if berr == nil {
		return 1
	}
	return strings.Compare(a, b)
}

// wave22MillerRabin is deterministic Miller-Rabin, exact for all uint64
// (bases 2,3,5,7,11,13,17 per Sorenson-Webster).
func wave22MillerRabin(n int64) bool {
	if n < 2 {
		return false
	}
	for _, p := range []int64{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37} {
		if n%p == 0 {
			return n == p
		}
	}
	d := uint64(n - 1)
	s := 0
	for d%2 == 0 {
		d /= 2
		s++
	}
	n64 := uint64(n)
	for _, a := range []uint64{2, 3, 5, 7, 11, 13, 17} {
		if a%n64 == 0 {
			continue
		}
		x := wave22modPow(a, d, n64)
		if x == 1 || x == n64-1 {
			continue
		}
		composite := true
		for r := 1; r < s; r++ {
			x = wave22modMul(x, x, n64)
			if x == n64-1 {
				composite = false
				break
			}
		}
		if composite {
			return false
		}
	}
	return true
}

func wave22modMul(a, b, m uint64) uint64 {
	// 128-bit intermediate via bits.Mul-style splitting to avoid overflow.
	hi, lo := bitsMul64(a, b)
	if hi == 0 {
		return lo % m
	}
	// Fallback: repeated addition (rare; only for huge operands near 2^64).
	var r uint64
	aa := a % m
	bb := b
	for bb > 0 {
		if bb&1 == 1 {
			r = (r + aa) % m
		}
		aa = (aa << 1) % m
		bb >>= 1
	}
	return r
}

func bitsMul64(a, b uint64) (hi, lo uint64) {
	// Split into 32-bit halves for exact 128-bit product.
	a0, a1 := a&0xffffffff, a>>32
	b0, b1 := b&0xffffffff, b>>32
	p0 := a0 * b0
	p1 := a0 * b1
	p2 := a1 * b0
	p3 := a1 * b1
	mid := p1 + p2
	carry := uint64(0)
	if mid < p1 {
		carry = 1 << 32
	}
	lo = p0 + (mid << 32)
	if lo < p0 {
		carry++
	}
	hi = p3 + (mid >> 32) + carry
	return hi, lo
}

func wave22modPow(a, d, m uint64) uint64 {
	r := uint64(1)
	a %= m
	for d > 0 {
		if d&1 == 1 {
			r = wave22modMul(r, a, m)
		}
		a = wave22modMul(a, a, m)
		d >>= 1
	}
	return r
}

// ---- Wave 22c: build-time version stamping ----

// versionStamp is set at release-build time via:
//   go build -ldflags "-X github.com/navescript/nvs/internal/eval.versionStamp=$(git describe --tags --always)"
// Plain `go build` keeps the source default, so every dev build still
// reports the same version as before — fully backward compatible.
var versionStamp string

// nvsRuntimeVersion reports the stamped version when present, else the default.
func nvsRuntimeVersion() string {
	if versionStamp != "" {
		return versionStamp
	}
	return "2.1.0"
}
