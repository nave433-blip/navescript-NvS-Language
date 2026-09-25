package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/navescript/nvs/internal/bytecode"
	"github.com/navescript/nvs/internal/nave"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
	"github.com/navescript/nvs/internal/tools"
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
	case "fmt":
		runFmt(os.Args[2:])
	case "lint":
		runLint(os.Args[2:])
	case "doc":
		runDoc(os.Args[2:])
	case "transpile":
		runTranspile(os.Args[2:])
	case "bridge":
		runBridge(os.Args[2:])
	case "bc":
		// Experimental bytecode compile+run, ported from the 2.8 track.
		// Honest subset: arithmetic, comparisons, let/const, if/else,
		// print. Anything else fails LOUDLY at compile time.
		runBytecode(os.Args[2:])
	case "nave":
		// Workflow runner ported from the 2.9 track: executes JSON
		// .nave workflow documents (log/set/http_get/file ops,
		// polyglot python/js steps, assertions).
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nvs nave <file.nave>")
			os.Exit(1)
		}
		if err := nave.RunFile(os.Args[2], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printUsage()
	default:
		if strings.HasSuffix(cmd, ".nave") {
			if err := nave.RunFile(cmd, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else if strings.HasSuffix(cmd, ".ns") {
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
  nvs fmt [files...]      Format files (reads stdin if no files)
  nvs lint [files...]     Lint files (0 = clean, 1 = findings)
  nvs doc [files...]      Extract doc comments as Markdown (minimal stub)
  nvs transpile --to=js|python <file.ns>
                          Transpile the NvS subset to JavaScript or Python
  nvs bridge              JSON stdio bridge: read requests on stdin,
                          write {"ok":...} responses on stdout
  nvs nave <file.nave>    Run a JSON workflow document (also: nvs file.nave)
  nvs bc <file.ns|--code> Compile the bytecode subset and run it on the
                          stack VM (--disasm to print bytecode). Only
                          arithmetic, let/const, if/else, print.
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

// --- Wave 9: developer tooling ---

func fmtHelp() {
	fmt.Print(`nvs fmt — canonical code formatter (lexical, gofmt/rustfmt idea)

Usage:
  nvs fmt [--check] [files...]
  nvs fmt --help

With no files, reads stdin and writes formatted output to stdout.
Otherwise formats each file in place.

Normalizes: 4-space indentation by brace/paren/bracket depth, tabs to
spaces (outside strings), trailing-whitespace removal, blank-line
collapsing (max 1 consecutive), exactly one trailing newline.
Leaves alone: string contents (incl. ${} interpolation), comments,
in-line spacing.

  --check   exit 1 and list files that would change; exit 0 if all clean
`)
}

func runFmt(args []string) {
	check := false
	var files []string
	for _, a := range args {
		switch a {
		case "--check":
			check = true
		case "-h", "--help":
			fmtHelp()
			return
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		if check {
			fmt.Fprintln(os.Stderr, "nvs fmt --check needs at least one file")
			os.Exit(1)
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fmt: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(tools.Format(string(data)))
		return
	}
	changedAny := false
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fmt: %v\n", err)
			os.Exit(1)
		}
		formatted := tools.Format(string(data))
		if formatted == string(data) {
			continue
		}
		changedAny = true
		if check {
			fmt.Println(f)
			continue
		}
		changed, err := tools.FormatFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fmt: %v\n", err)
			os.Exit(1)
		}
		if changed {
			fmt.Printf("formatted %s\n", f)
		}
	}
	if check && changedAny {
		os.Exit(1)
	}
}

func lintHelp() {
	fmt.Print(`nvs lint — small set of sound static checks (clippy idea)

Usage:
  nvs lint [--json] <files...>
  nvs lint --help

Rules:
  unused-binding  let/const bound but never referenced (warning)
                  (function params excluded; named fns exempt)
  shadow-builtin  let/const/assignment shadows a builtin like len (warning)
  unreachable-code  code after return/break/continue/throw in a block (warning)
  null-comparison   x == null / x != null; suggests is_null() (style)

Findings print as: file:line: severity rule: message
Exit code: 0 = clean, 1 = findings (or a file that fails to parse).

  --json   emit findings as JSON instead of text
`)
}

func runLint(args []string) {
	asJSON := false
	var files []string
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			lintHelp()
			return
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nvs lint [--json] <files...>")
		os.Exit(1)
	}
	var all []tools.Finding
	for _, f := range files {
		findings, perr, err := tools.LintFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lint: %v\n", err)
			os.Exit(1)
		}
		for _, e := range perr {
			fmt.Fprintf(os.Stderr, "%s: parser error: %s\n", f, e)
		}
		all = append(all, findings...)
	}
	if asJSON {
		fmt.Print(tools.FindingsJSON(all))
	} else {
		for _, fd := range all {
			fmt.Println(fd.String())
		}
	}
	if len(all) > 0 {
		os.Exit(1)
	}
}

func docHelp() {
	fmt.Print(`nvs doc — MINIMAL doc-comment extractor (stub, not rustdoc)

Usage:
  nvs doc <files...>
  nvs doc --help

Extracts // doc comments immediately preceding top-level fn/class/record/
interface declarations (and fn params, trivially) and emits Markdown:

  ## name
  ` + "`fn name(a, b)`" + `
  doc text...

Only top-level declarations are covered; no nested docs, no cross-links.
`)
}

func runDoc(args []string) {
	var files []string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			docHelp()
			return
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nvs doc <files...>")
		os.Exit(1)
	}
	multi := len(files) > 1
	for _, f := range files {
		entries, perr, err := tools.ExtractDocsFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "doc: %v\n", err)
			os.Exit(1)
		}
		for _, e := range perr {
			fmt.Fprintf(os.Stderr, "%s: parser error: %s\n", f, e)
		}
		fmt.Print(tools.RenderMarkdown(f, entries, multi))
	}
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

// runBytecode compiles the supported subset to bytecode and runs it on the
// stack VM. Anything outside the subset is a loud compile error, never
// silently wrong output.
func runBytecode(args []string) {
	disasm := false
	var code string
	for _, a := range args {
		switch a {
		case "--disasm", "-d":
			disasm = true
		default:
			if strings.HasSuffix(a, ".ns") {
				data, err := os.ReadFile(a)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error reading %s: %v\n", a, err)
					os.Exit(1)
				}
				code = string(data)
			} else {
				code = a
			}
		}
	}
	if code == "" {
		fmt.Fprintln(os.Stderr, "usage: nvs bc <file.ns|'code'> [--disasm]")
		os.Exit(1)
	}
	l := lexer.New(code)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		printParserErrors(os.Stderr, p.Errors())
		os.Exit(1)
	}
	c := bytecode.NewCompiler()
	if err := c.Compile(program); err != nil {
		fmt.Fprintf(os.Stderr, "bytecode compile error: %v\n", err)
		os.Exit(1)
	}
	bc := c.Bytecode()
	if disasm {
		fmt.Print(bytecode.Disassemble(bc))
	}
	vm := bytecode.NewVM(bc)
	if _, err := vm.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bytecode runtime error: %v\n", err)
		os.Exit(1)
	}
}
