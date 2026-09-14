package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

const VERSION = "1.3.0-working"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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
		runFile(os.Args[2])
	case "eval", "e":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: nvs eval '<code>'")
			os.Exit(1)
		}
		code := strings.Join(os.Args[2:], " ")
		runCode(code)
	case "repl", "i":
		startREPL()
	case "version", "-v", "--version":
		fmt.Printf("Navescript (NvS) %s\n", VERSION)
	case "help", "-h", "--help":
		printUsage()
	default:
		if strings.HasSuffix(cmd, ".ns") || strings.HasSuffix(cmd, ".nave") {
			runFile(cmd)
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Print(`Navescript (NvS) - Minimal Working Runtime

Usage:
  nvs run <file.ns>     Run a Navescript file
  nvs eval '<code>'     Evaluate code string
  nvs repl              Start interactive REPL
  nvs version           Show version
  nvs help              Show this help

Examples:
  nvs run hello.ns
  nvs eval 'print 1 + 2 * 3'
  nvs eval 'let x = 10; print x * 2'
`)
}

func runFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		os.Exit(1)
	}
	runCode(string(data))
}

func runCode(code string) {
	env := object.NewEnvironment()
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
}

func printParserErrors(out io.Writer, errors []string) {
	fmt.Fprintln(out, "Parser errors:")
	for _, msg := range errors {
		fmt.Fprintf(out, "  %s\n", msg)
	}
}

func startREPL() {
	fmt.Printf("Navescript (NvS) %s REPL\n", VERSION)
	fmt.Println("Type expressions or statements. Ctrl+D to exit.")
	env := object.NewEnvironment()

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
