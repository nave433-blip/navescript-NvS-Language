package polyglot

import (
	"os/exec"
	"strings"
)

// Capability flags for an applet (language runtime plugin).
const (
	CapRun        = "run"        // execute code
	CapCompile    = "compile"    // compile to native/binary
	CapTranslate  = "translate"  // bidirectional simple translate ↔ NvS
	CapDetect     = "detect"     // participate in auto-detection
	CapInteroper  = "interop"    // can exchange values via stdout/JSON bridge
)

// Applet describes a host language runtime and how NvS talks to it.
type Applet struct {
	ID          string   // e.g. "python"
	Name        string   // display name
	Extensions  []string // .py, .rs, ...
	Binaries    []string // required host tools
	Capabilities []string
	Aliases     []string
	Run         func(code string) Result
	// ToNvS best-effort translates native snippet → NvS source
	ToNvS func(code string) (string, error)
	// FromNvS best-effort translates NvS snippet → native source
	FromNvS func(code string) (string, error)
}

var registry = map[string]*Applet{}

func init() {
	registerDefaults()
}

func register(a *Applet) {
	registry[a.ID] = a
	for _, al := range a.Aliases {
		registry[al] = a
	}
}

func registerDefaults() {
	register(&Applet{
		ID: "python", Name: "Python", Extensions: []string{".py"},
		Binaries: []string{"python3"}, Aliases: []string{"py", "python3"},
		Capabilities: []string{CapRun, CapDetect, CapTranslate, CapInteroper},
		Run: Python,
		ToNvS: translatePythonToNvS, FromNvS: translateNvSToPython,
	})
	register(&Applet{
		ID: "js", Name: "JavaScript", Extensions: []string{".js", ".mjs"},
		Binaries: []string{"node"}, Aliases: []string{"javascript", "node"},
		Capabilities: []string{CapRun, CapDetect, CapTranslate, CapInteroper},
		Run: JS,
		ToNvS: translateJSToNvS, FromNvS: translateNvSToJS,
	})
	register(&Applet{
		ID: "ruby", Name: "Ruby", Extensions: []string{".rb"},
		Binaries: []string{"ruby"}, Aliases: []string{"rb"},
		Capabilities: []string{CapRun, CapDetect, CapTranslate, CapInteroper},
		Run: Ruby,
		ToNvS: translateRubyToNvS, FromNvS: translateNvSToRuby,
	})
	register(&Applet{
		ID: "rust", Name: "Rust", Extensions: []string{".rs"},
		Binaries: []string{"rustc"}, Aliases: []string{"rs"},
		Capabilities: []string{CapRun, CapCompile, CapDetect, CapTranslate, CapInteroper},
		Run: Rust,
		ToNvS: translateRustToNvS, FromNvS: translateNvSToRust,
	})
	register(&Applet{
		ID: "go", Name: "Go", Extensions: []string{".go"},
		Binaries: []string{"go"}, Aliases: []string{"golang"},
		Capabilities: []string{CapRun, CapCompile, CapDetect, CapTranslate, CapInteroper},
		Run: Go,
		ToNvS: translateGoToNvS, FromNvS: translateNvSToGo,
	})
	register(&Applet{
		ID: "c", Name: "C", Extensions: []string{".c", ".h"},
		Binaries: []string{"gcc"}, Aliases: []string{"gcc"},
		Capabilities: []string{CapRun, CapCompile, CapDetect, CapTranslate, CapInteroper},
		Run: C,
		ToNvS: translateCToNvS, FromNvS: translateNvSToC,
	})
	register(&Applet{
		ID: "cpp", Name: "C++", Extensions: []string{".cpp", ".cc", ".cxx", ".hpp"},
		Binaries: []string{"g++"}, Aliases: []string{"c++", "cxx"},
		Capabilities: []string{CapRun, CapCompile, CapDetect, CapTranslate, CapInteroper},
		Run: CPP,
		ToNvS: translateCppToNvS, FromNvS: translateNvSToCpp,
	})
	register(&Applet{
		ID: "java", Name: "Java", Extensions: []string{".java"},
		Binaries: []string{"javac", "java"}, Aliases: []string{"jvm"},
		Capabilities: []string{CapRun, CapCompile, CapDetect, CapTranslate, CapInteroper},
		Run: Java,
		ToNvS: translateJavaToNvS, FromNvS: translateNvSToJava,
	})
	register(&Applet{
		ID: "css", Name: "CSS", Extensions: []string{".css"},
		Binaries: nil, Aliases: []string{},
		Capabilities: []string{CapRun, CapDetect},
		Run: CSS,
	})
	register(&Applet{
		ID: "nvs", Name: "Navescript", Extensions: []string{".ns", ".nave"},
		Binaries: nil, Aliases: []string{"navescript"},
		Capabilities: []string{CapDetect, CapTranslate, CapInteroper},
		Run: func(code string) Result {
			return Result{Output: code, Err: nil} // identity; real eval is host NvS
		},
		ToNvS: func(code string) (string, error) { return code, nil },
		FromNvS: func(code string) (string, error) { return code, nil },
	})
}

// GetApplet resolves an applet by id or alias.
func GetApplet(id string) (*Applet, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	a, ok := registry[id]
	return a, ok
}

// ListApplets returns unique applets (no alias duplicates).
func ListApplets() []*Applet {
	seen := map[string]bool{}
	var out []*Applet
	for _, a := range registry {
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		out = append(out, a)
	}
	return out
}

// AppletReady reports whether required binaries exist.
func AppletReady(a *Applet) bool {
	if a == nil {
		return false
	}
	if len(a.Binaries) == 0 {
		return true
	}
	for _, b := range a.Binaries {
		if _, err := exec.LookPath(b); err != nil {
			return false
		}
	}
	return true
}

// ChooseApplet picks the best applet for explicit lang or auto-detect from code.
func ChooseApplet(langHint, code string) (*Applet, string) {
	if langHint != "" {
		if a, ok := GetApplet(langHint); ok {
			return a, "explicit:" + a.ID
		}
	}
	id, conf := DetectLanguage(code)
	if a, ok := GetApplet(id); ok {
		return a, "detect:" + id + "@" + conf
	}
	return nil, "none"
}
