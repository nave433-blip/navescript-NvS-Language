package polyglot

import (
	"fmt"
	"regexp"
	"strings"
)

// --- Native → NvS (simple idioms) ---

func translatePythonToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`print\((.+)\)`:               `print $1`,
		`(?m)^\s*def\s+(\w+)\s*\(([^)]*)\):`: `fn $1($2) {`,
		`(?m)^\s*#\s*(.*)$`:           `// $1`,
		`True`:                        `true`,
		`False`:                       `false`,
		`None`:                        `null`,
		`(?m)^\s*elif\s+`:             `else if `,
	})
}

func translateJSToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`console\.log\((.+)\)`:        `print $1`,
		`\bconst\s+`:                  `const `,
		`\blet\s+`:                    `let `,
		`\bvar\s+`:                    `let `,
		`\bfunction\s+(\w+)\s*\(([^)]*)\)`: `fn $1($2)`,
		`===`:                         `==`,
		`!==`:                         `!=`,
		`\bnull\b`:                    `null`,
		`\bundefined\b`:               `null`,
	})
}

func translateRubyToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`puts\s+(.+)`:                 `print $1`,
		`(?m)^\s*def\s+(\w+)(?:\(([^)]*)\))?`: `fn $1($2) {`,
		`(?m)^\s*end\s*$`:             `}`,
		`true`:                        `true`,
		`false`:                       `false`,
		`nil`:                         `null`,
	})
}

func translateRustToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`println!\("([^"]*)"\)`:       `print "$1"`,
		`println!\((.+)\)`:            `print $1`,
		`\blet\s+mut\s+`:              `let `,
		`\blet\s+`:                    `let `,
		`\bfn\s+(\w+)\s*\(([^)]*)\)`:  `fn $1($2)`,
		`true`:                        `true`,
		`false`:                       `false`,
	})
}

func translateGoToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`fmt\.Println\((.+)\)`:        `print $1`,
		`fmt\.Print\((.+)\)`:          `print $1`,
		`\bfunc\s+(\w+)\s*\(([^)]*)\)`: `fn $1($2)`,
		`:=`:                          `=`,
		`true`:                        `true`,
		`false`:                       `false`,
		`nil`:                         `null`,
	})
}

func translateCToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`printf\("([^"]*)\\n"\)`:      `print "$1"`,
		`printf\("([^"]*)"\)`:         `print "$1"`,
		`\bint\s+(\w+)\s*=\s*`:        `let $1 = `,
		`\btrue\b`:                    `true`,
		`\bfalse\b`:                   `false`,
	})
}

func translateCppToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`std::cout\s*<<\s*"([^"]*)"\s*<<\s*std::endl`: `print "$1"`,
		`cout\s*<<\s*"([^"]*)"`:       `print "$1"`,
		`\btrue\b`:                    `true`,
		`\bfalse\b`:                   `false`,
	})
}

func translateJavaToNvS(code string) (string, error) {
	return genericToNvS(code, map[string]string{
		`System\.out\.println\((.+)\)`: `print $1`,
		`System\.out\.print\((.+)\)`:  `print $1`,
		`\btrue\b`:                    `true`,
		`\bfalse\b`:                   `false`,
		`\bnull\b`:                    `null`,
	})
}

// --- NvS → Native ---

func translateNvSToPython(code string) (string, error) {
	return genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+(.+)$`:       `print($1)`,
		`(?m)^\s*fn\s+(\w+)\s*\(([^)]*)\)\s*\{`: `def $1($2):`,
		`(?m)^\s*let\s+`:              ``,
		`(?m)^\s*const\s+`:            ``,
		`\btrue\b`:                    `True`,
		`\bfalse\b`:                   `False`,
		`\bnull\b`:                    `None`,
		`//`:                          `#`,
	})
}

func translateNvSToJS(code string) (string, error) {
	return genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+(.+)$`:       `console.log($1);`,
		`(?m)^\s*fn\s+(\w+)\s*\(([^)]*)\)\s*\{`: `function $1($2) {`,
		`(?m)^\s*let\s+`:              `let `,
		`(?m)^\s*const\s+`:            `const `,
		`\bnull\b`:                    `null`,
	})
}

func translateNvSToRuby(code string) (string, error) {
	return genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+(.+)$`:       `puts $1`,
		`(?m)^\s*fn\s+(\w+)\s*\(([^)]*)\)\s*\{`: `def $1($2)`,
		`(?m)^\s*\}\s*$`:              `end`,
		`(?m)^\s*let\s+`:              ``,
		`\bnull\b`:                    `nil`,
	})
}

func translateNvSToRust(code string) (string, error) {
	body, err := genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+"([^"]*)"$`:  `println!("$1");`,
		`(?m)^\s*print\s+(.+)$`:       `println!("{}", $1);`,
		`(?m)^\s*let\s+(\w+)\s*=`:     `let $1 =`,
		`(?m)^\s*fn\s+(\w+)\s*\(([^)]*)\)\s*\{`: `fn $1($2) {`,
	})
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, "fn main") {
		body = "fn main() {\n" + body + "\n}\n"
	}
	return body, nil
}

func translateNvSToGo(code string) (string, error) {
	body, err := genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+(.+)$`:       `fmt.Println($1)`,
		`(?m)^\s*let\s+(\w+)\s*=`:     `$1 :=`,
		`(?m)^\s*fn\s+(\w+)\s*\(([^)]*)\)\s*\{`: `func $1($2) {`,
		`\bnull\b`:                    `nil`,
	})
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, "package ") {
		body = "package main\nimport \"fmt\"\nfunc main() {\n" + body + "\n}\n"
	}
	return body, nil
}

func translateNvSToC(code string) (string, error) {
	body, err := genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+"([^"]*)"$`:  `printf("$1\\n");`,
		`(?m)^\s*let\s+(\w+)\s*=\s*(\d+)`: `int $1 = $2;`,
	})
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, "main(") {
		body = "#include <stdio.h>\nint main(void) {\n" + body + "\nreturn 0;\n}\n"
	}
	return body, nil
}

func translateNvSToCpp(code string) (string, error) {
	body, err := genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+"([^"]*)"$`:  `std::cout << "$1" << std::endl;`,
		`(?m)^\s*let\s+(\w+)\s*=\s*(\d+)`: `int $1 = $2;`,
	})
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, "main(") {
		body = "#include <iostream>\nint main() {\n" + body + "\nreturn 0;\n}\n"
	}
	return body, nil
}

func translateNvSToJava(code string) (string, error) {
	body, err := genericFromNvS(code, map[string]string{
		`(?m)^\s*print\s+(.+)$`:       `System.out.println($1);`,
		`(?m)^\s*let\s+(\w+)\s*=\s*(\d+)`: `int $1 = $2;`,
	})
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, "class ") {
		body = "public class Main {\n  public static void main(String[] args) {\n" + body + "\n  }\n}\n"
	}
	return body, nil
}

// genericToNvS applies ordered regex replacements (keys are patterns).
func genericToNvS(code string, rules map[string]string) (string, error) {
	out := code
	for pat, repl := range rules {
		re, err := regexp.Compile(pat)
		if err != nil {
			return "", fmt.Errorf("translate pattern %q: %w", pat, err)
		}
		out = re.ReplaceAllString(out, repl)
	}
	return out, nil
}

func genericFromNvS(code string, rules map[string]string) (string, error) {
	return genericToNvS(code, rules)
}

// Translate converts code from → to language ids using registered applets.
// Real-time corrections from the DB are applied when present.
func Translate(fromLang, toLang, code string) (string, error) {
	fromLang = strings.ToLower(strings.TrimSpace(fromLang))
	toLang = strings.ToLower(strings.TrimSpace(toLang))
	if fromLang == toLang {
		return code, nil
	}
	// Real-time pull from corrections DB
	if c, ok := PullCorrection(fromLang, toLang, code); ok && c.Corrected != "" {
		return c.Corrected, nil
	}
	// Path via NvS as intermediate for cross-language pairs
	var nvsCode string
	var err error
	if fromLang == "nvs" || fromLang == "navescript" {
		nvsCode = code
	} else {
		from, ok := GetApplet(fromLang)
		if !ok || from.ToNvS == nil {
			return "", fmt.Errorf("no translator from %s to nvs", fromLang)
		}
		nvsCode, err = from.ToNvS(code)
		if err != nil {
			return "", err
		}
	}
	if toLang == "nvs" || toLang == "navescript" {
		return nvsCode, nil
	}
	to, ok := GetApplet(toLang)
	if !ok || to.FromNvS == nil {
		return "", fmt.Errorf("no translator from nvs to %s", toLang)
	}
	return to.FromNvS(nvsCode)
}
