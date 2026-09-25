package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// NvS — Navescript custom language
const (
	VERSION      = "2.1.0"
	LANGUAGE     = "NvS"
	LANGUAGEFull = "Navescript"
)

func main() {
	if len(os.Args) < 2 {
		startREPL()
		return
	}

	cmd := os.Args[1]
	eval.CLIArgs = os.Args[1:]
	switch cmd {
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nvs run <file.ns>")
			os.Exit(1)
		}
		eval.CLIArgs = os.Args[3:]
		runFile(os.Args[2], true)
	case "eval", "e":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nvs eval '<code>'")
			os.Exit(1)
		}
		code := strings.Join(os.Args[2:], " ")
		runCode(code, true)
	case "repl", "i":
		startREPL()
	case "init":
		initProject(".")
	case "version", "-v", "--version":
		fmt.Printf("%s (%s) %s\n", LANGUAGE, LANGUAGEFull, VERSION)
	case "info":
		printLanguageInfo()
	case "help", "-h", "--help":
		printUsage()
	default:
		if strings.HasSuffix(cmd, ".ns") || strings.HasSuffix(cmd, ".nave") {
			runFile(cmd, true)
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Printf(`%s (%s) %s — a custom scripting language

Usage:
  nvs                     Start interactive REPL
  nvs run <file.ns>       Run a program
  nvs eval '<code>'       Evaluate a snippet
  nvs init                Scaffold a new NvS project
  nvs info                Language identity & capabilities
  nvs version             Show version
  nvs help                Show this help

File extensions: .ns  .nave

Examples:
  nvs run hello.ns
  nvs eval 'print 1 + 2 * 3'
  nvs
`, LANGUAGE, LANGUAGEFull, VERSION)
}

func printLanguageInfo() {
	fmt.Printf(`Language:     %s (%s)
Version:      %s
Paradigm:     multi (imperative, functional, OOP)
Typing:       dynamic (optional runtime annotations)
Implementation: tree-walking interpreter (Go host)
Extensions:   .ns, .nave
Stdlib:       prelude, polyglot, highlight, fuzzy, corrections

Core features:
  let / const / null / ?? / and|or|&&||
  functions (defaults, closures, generators)
  classes / new / this / extends
  match|switch, try/catch/throw
  modules (import), higher-order builtins
  polyglot: python js ruby rust go c cpp java css
  interop: detect_lang, to_nvs, from_nvs, translate
  highlight(), fuzzy_*(), corrections DB
`, LANGUAGE, LANGUAGEFull, VERSION)
}

func initProject(dir string) {
	files := map[string]string{
		"main.ns": `// NvS project entry
print "Hello from " + nvs_language() + " " + nvs_version()

fn main() {
  print "NvS is ready."
}

main()
`,
		"nvs.json": fmt.Sprintf(`{
  "name": "nvs-app",
  "version": "0.1.0",
  "language": "%s",
  "nvs": "%s",
  "main": "main.ns"
}
`, LANGUAGE, VERSION),
		"README.md": "# NvS project\n\n```bash\nnvs run main.ns\n```\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("skip existing %s\n", path)
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "init: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("created %s\n", path)
	}
	fmt.Println("NvS project initialized. Run: nvs run main.ns")
}

func runFile(path string, withPrelude bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		os.Exit(1)
	}
	// Set working directory context for imports relative to file
	eval.CurrentFile = path
	runCode(string(data), withPrelude)
}

func runCode(code string, withPrelude bool) {
	env := object.NewEnvironment()
	if withPrelude {
		eval.LoadPrelude(env)
	}
	l := lexer.New(code)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		printParserErrors(os.Stderr, p.Errors())
		os.Exit(1)
	}

	result := eval.Eval(program, env)
	if result != nil && result.Type() == object.ERROR_OBJ {
		fmt.Fprintln(os.Stderr, result.Inspect())
		os.Exit(1)
	}
	// Wave 6: wait for all spawned tasks to finish before exiting, so no
	// task output is lost to an early exit. A program that raised an error
	// exits immediately above instead of risking a deadlock here.
	eval.DrainSpawnedTasks()
}

func printParserErrors(out io.Writer, errors []string) {
	fmt.Fprintln(out, "NvS parser errors:")
	for _, msg := range errors {
		fmt.Fprintf(out, "  %s\n", msg)
	}
}

func startREPL() {
	fmt.Printf("%s (%s) %s\n", LANGUAGE, LANGUAGEFull, VERSION)
	fmt.Println("Interactive mode. Type expressions; exit or Ctrl+D to quit.")
	env := object.NewEnvironment()
	eval.LoadPrelude(env)

	buf := make([]byte, 4096)
	for {
		fmt.Print("nvs> ")
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(string(buf[:n]))
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return
		}

		l := lexer.New(line)
		p := parser.New(l)
		program := p.ParseProgram()
		if len(p.Errors()) > 0 {
			printParserErrors(os.Stdout, p.Errors())
			continue
		}

		result := eval.Eval(program, env)
		if result != nil && result.Type() != object.NULL_OBJ {
			fmt.Println(result.Inspect())
		}
	}
}
