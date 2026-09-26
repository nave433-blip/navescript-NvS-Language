// Package nave runs NJSON/.nave workflow documents (Jarvis / system_health style).
package nave

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/navescript/nvs/internal/polyglot"
)

type Doc struct {
	Version string                   `json:"version"`
	NJSON   string                   `json:"njson_version"`
	Module  string                   `json:"module"`
	World   string                   `json:"world"`
	Steps   []map[string]interface{} `json:"steps"`
}

type Runner struct {
	Vars   map[string]interface{}
	Out    io.Writer
	DryRun bool
}

func NewRunner(out io.Writer) *Runner {
	if out == nil {
		out = os.Stdout
	}
	return &Runner{Vars: map[string]interface{}{}, Out: out}
}

func Parse(data []byte) (*Doc, error) {
	var d Doc
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Runner) logf(format string, args ...interface{}) {
	fmt.Fprintf(r.Out, format+"\n", args...)
}

func (r *Runner) resolve(v interface{}) interface{} {
	s, ok := v.(string)
	if !ok {
		return v
	}
	// config.field or bare var
	if strings.Contains(s, ".") {
		parts := strings.SplitN(s, ".", 2)
		if base, ok := r.Vars[parts[0]]; ok {
			if m, ok := base.(map[string]interface{}); ok {
				if val, ok := m[parts[1]]; ok {
					return val
				}
			}
		}
	}
	if val, ok := r.Vars[s]; ok {
		return val
	}
	return s
}

func (r *Runner) Run(doc *Doc) error {
	r.logf("nave: module=%s version=%s world=%s steps=%d",
		doc.Module, first(doc.Version, doc.NJSON), doc.World, len(doc.Steps))
	i := 0
	for i < len(doc.Steps) {
		step := doc.Steps[i]
		op, _ := step["op"].(string)
		if op == "" {
			op, _ = step["type"].(string)
		}
		switch op {
		case "log":
			msg := step["message"]
			if msg == nil {
				if p, ok := step["params"].(map[string]interface{}); ok {
					msg = p["message"]
				}
			}
			r.logf("%v", r.resolve(msg))
		case "set":
			name, _ := step["var"].(string)
			r.Vars[name] = step["value"]
		case "answer":
			name, _ := step["var"].(string)
			r.logf("answer %s = %v", name, r.Vars[name])
		case "input":
			// non-interactive: use value if provided, else empty / preset
			name, _ := step["return_var"].(string)
			if name == "" {
				name, _ = step["var"].(string)
			}
			if _, ok := r.Vars[name]; !ok {
				r.Vars[name] = "status" // default non-interactive command
			}
			r.logf("input %s => %v (non-interactive)", name, r.Vars[name])
		case "polyglot_eval":
			if err := r.polyglot(step); err != nil {
				r.logf("polyglot_eval error: %v", err)
				// continue unless hard fail
			}
		case "http_get":
			url, _ := step["url"].(string)
			if url == "" {
				url, _ = r.resolve(step["url"]).(string)
			}
			url = fmt.Sprint(r.resolve(url))
			ret, _ := step["return_var"].(string)
			body, err := httpGet(url)
			if err != nil {
				r.logf("http_get error: %v", err)
				r.Vars[ret] = map[string]interface{}{"error": err.Error()}
			} else {
				r.Vars[ret] = body
				r.logf("http_get %s => %d bytes", url, len(body))
			}
		case "file_read":
			path := fmt.Sprint(r.resolve(step["path"]))
			ret, _ := step["return_var"].(string)
			b, err := os.ReadFile(path)
			if err != nil {
				r.Vars[ret] = map[string]interface{}{"error": err.Error()}
				r.logf("file_read error: %v", err)
			} else {
				r.Vars[ret] = string(b)
				r.logf("file_read %s ok", path)
			}
		case "file_write":
			path := fmt.Sprint(r.resolve(step["path"]))
			content := fmt.Sprint(r.resolve(step["content"]))
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				r.logf("file_write error: %v", err)
			} else {
				r.logf("file_write %s ok", path)
			}
		case "if":
			// minimal: check var truthiness or skip then branch
			cond := step["condition"]
			ok := truthy(r.resolve(cond))
			// steps may embed "then" / "else" arrays
			branch := "then"
			if !ok {
				branch = "else"
			}
			if sub, ok := step[branch].([]interface{}); ok {
				for _, raw := range sub {
					if m, ok := raw.(map[string]interface{}); ok {
						// run nested by temporarily injecting
						doc2 := &Doc{Steps: []map[string]interface{}{m}}
						_ = r.Run(doc2)
					}
				}
			}
			r.logf("if condition=%v branch=%s", ok, branch)
		case "try":
			// run try steps; ignore errors for stage-1
			if sub, ok := step["steps"].([]interface{}); ok {
				for _, raw := range sub {
					if m, ok := raw.(map[string]interface{}); ok {
						_ = r.Run(&Doc{Steps: []map[string]interface{}{m}})
					}
				}
			}
		case "nasm_exec", "component_call":
			r.logf("skip unsupported op %s (stub)", op)
		case "native_op":
			// Small arithmetic/logic ops over resolved vars.
			// (Port note: the 2.9 track skipped these; supporting the
			// basic set makes the bundled examples run end-to-end.)
			operator, _ := step["operator"].(string)
			args, _ := step["args"].([]interface{})
			ret, _ := step["return_var"].(string)
			vals := make([]float64, 0, len(args))
			for _, a := range args {
				vals = append(vals, toFloat64(r.resolve(a)))
			}
			var result interface{}
			switch strings.ToLower(operator) {
			case "add":
				result = vals[0] + vals[1]
			case "sub":
				result = vals[0] - vals[1]
			case "mul":
				result = vals[0] * vals[1]
			case "div":
				if vals[1] == 0 {
					result = map[string]interface{}{"error": "div by zero"}
				} else {
					result = vals[0] / vals[1]
				}
			case "eq":
				result = fmt.Sprint(r.resolve(args[0])) == fmt.Sprint(r.resolve(args[1]))
			default:
				r.logf("skip unsupported native_op %q", operator)
			}
			if result != nil && ret != "" {
				r.Vars[ret] = result
			}
		case "assert_eq":
			a := r.resolve(step["left"])
			b := r.resolve(step["right"])
			if fmt.Sprint(a) != fmt.Sprint(b) {
				return fmt.Errorf("assert_eq failed: %v != %v", a, b)
			}
			r.logf("assert_eq ok")
		default:
			r.logf("skip unknown op %q", op)
		}
		i++
	}
	r.logf("nave: done module=%s", doc.Module)
	return nil
}

func (r *Runner) polyglot(step map[string]interface{}) error {
	lang, _ := step["lang"].(string)
	code, _ := step["code"].(string)
	ret, _ := step["return_var"].(string)
	inVar, _ := step["input_var"].(string)

	// Inject input_data for JS/Python snippets
	input := r.Vars[inVar]
	switch strings.ToLower(lang) {
	case "python", "py":
		pre := "input_data = None\nresult = None\n"
		if input != nil {
			b, _ := json.Marshal(input)
			pre = fmt.Sprintf("import json\ninput_data = json.loads(%q)\nresult = None\n", string(b))
		}
		post := "\nimport json\nprint(json.dumps(result) if result is not None else 'null')\n"
		res := polyglot.Python(pre + code + post)
		if res.Err != nil {
			return fmt.Errorf("%v: %s", res.Err, res.Output)
		}
		var out interface{}
		_ = json.Unmarshal([]byte(strings.TrimSpace(res.Output)), &out)
		r.Vars[ret] = out
		r.logf("polyglot python → %s", ret)
	case "javascript", "js":
		pre := "var input_data = null; var result = null;\n"
		if input != nil {
			b, _ := json.Marshal(input)
			pre = fmt.Sprintf("var input_data = %s; var result = null;\n", string(b))
		}
		post := "\nconsole.log(JSON.stringify(result));\n"
		res := polyglot.JS(pre + code + post)
		if res.Err != nil {
			// try node if available via same
			return fmt.Errorf("%v: %s", res.Err, res.Output)
		}
		var out interface{}
		_ = json.Unmarshal([]byte(strings.TrimSpace(res.Output)), &out)
		r.Vars[ret] = out
		r.logf("polyglot js → %s", ret)
	default:
		r.logf("polyglot skip unsupported lang %s", lang)
	}
	return nil
}

func httpGet(url string) (string, error) {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

func truthy(v interface{}) bool {
	switch n := v.(type) {
	case nil:
		return false
	case bool:
		return n
	case string:
		return n != "" && n != "0" && n != "false"
	case float64:
		return n != 0
	default:
		return true
	}
}

// toFloat64 coerces JSON-ish numbers for native_op arithmetic.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		var f float64
		_, _ = fmt.Sscanf(n, "%f", &f)
		return f
	default:
		return 0
	}
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// RunFile loads and executes a .nave document.
func RunFile(path string, out io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc, err := Parse(data)
	if err != nil {
		return err
	}
	return NewRunner(out).Run(doc)
}
