package polyglot

import (
	"fmt"
	"os/exec"
	"strings"
)

// NvSValidator is optionally set by the host (eval) to parse-check NvS code.
// Avoids import cycles between polyglot and parser.
var NvSValidator func(code string) error

// CheckResult is the outcome of translation + validation.
type CheckResult struct {
	OK          bool
	FromLang    string
	ToLang      string
	Source      string
	Output      string
	Errors      []string
	Corrected   bool // true if a DB correction was applied
	CorrectionID string
}

// ValidateOutput checks translated code for the target language.
func ValidateOutput(lang, code string) []string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	var errs []string
	code = strings.TrimSpace(code)
	if code == "" {
		return []string{"empty translation output"}
	}
	switch lang {
	case "nvs", "navescript":
		if NvSValidator != nil {
			if err := NvSValidator(code); err != nil {
				errs = append(errs, err.Error())
			}
		} else {
			// lightweight heuristic
			if strings.Count(code, "{") != strings.Count(code, "}") {
				errs = append(errs, "unbalanced braces in NvS output")
			}
			if strings.Count(code, "(") != strings.Count(code, ")") {
				errs = append(errs, "unbalanced parens in NvS output")
			}
		}
	case "python":
		r := runCmd("python3", "-c", "import ast,sys; ast.parse(sys.stdin.read())", code)
		// python -c with stdin: use different approach
		r = runPythonSyntax(code)
		if r.Err != nil {
			errs = append(errs, strings.TrimSpace(r.Output+r.Err.Error()))
		}
	case "js", "javascript":
		r := runCmd("node", "--check", code)
		// node --check needs a file; use -e with Function
		r = runCmd("node", "-e", "try{ new Function(process.argv[1]) }catch(e){ console.error(e.message); process.exit(1)}", code)
		if r.Err != nil {
			errs = append(errs, strings.TrimSpace(r.Output))
		}
	case "ruby":
		r := runCmd("ruby", "-c", "-e", code)
		if r.Err != nil {
			errs = append(errs, strings.TrimSpace(r.Output))
		}
	case "go", "golang":
		// gofmt or go/types would need a file; use go run dry via temp in Compile-less check
		if !strings.Contains(code, "package ") {
			errs = append(errs, "go output missing package clause (may still wrap at run)")
		}
	case "rust", "c", "cpp", "java":
		// structural balance only (full compile is expensive; run_native does that)
		if strings.Count(code, "{") != strings.Count(code, "}") {
			errs = append(errs, "unbalanced braces")
		}
		if strings.Count(code, "(") != strings.Count(code, ")") {
			errs = append(errs, "unbalanced parens")
		}
	default:
		if strings.Count(code, "{") != strings.Count(code, "}") {
			errs = append(errs, "unbalanced braces")
		}
	}
	return errs
}

func runPythonSyntax(code string) Result {
	cmd := exec.Command(PythonBinary(), "-c", "import ast,sys; ast.parse(sys.argv[1])", code)
	out, err := cmd.CombinedOutput()
	return Result{Output: string(out), Err: err}
}

// TranslateChecked translates, applies corrections, validates, and returns a report.
func TranslateChecked(from, to, code string) CheckResult {
	cr := CheckResult{FromLang: from, ToLang: to, Source: code}

	if c, ok := PullCorrection(from, to, code); ok && c.Corrected != "" {
		cr.Output = c.Corrected
		cr.Corrected = true
		cr.CorrectionID = c.ID
	} else {
		out, err := Translate(from, to, code)
		if err != nil {
			cr.OK = false
			cr.Errors = []string{err.Error()}
			return cr
		}
		cr.Output = out
	}

	errs := ValidateOutput(to, cr.Output)
	if len(errs) > 0 {
		cr.OK = false
		cr.Errors = errs
		return cr
	}
	cr.OK = true
	return cr
}

// AutoLearn stores a correction when user provides fixed output after a failed check.
func AutoLearn(from, to, source, bad, fixed, note string) (Correction, error) {
	if strings.TrimSpace(fixed) == "" {
		return Correction{}, fmt.Errorf("corrected output is empty")
	}
	return AddCorrection(from, to, source, fixed, bad, note)
}
