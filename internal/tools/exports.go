// Package tools: nvs exports — static analysis of an NvS file's public surface.
//
// `nvs exports <file.ns>` parses the file (without running it) and prints a
// JSON document describing the top-level bindings a foreign client can use:
// functions (names, parameters, defaults, return annotations), classes
// (methods), enums, and constants. This is how non-NvS tooling "reads" NvS.
package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// ParamExport describes one function/method parameter.
type ParamExport struct {
	Name     string  `json:"name"`
	Default  *string `json:"default,omitempty"` // rendered NvS source of the default, if any
	Type     *string `json:"type,omitempty"`    // rendered type annotation, if any
	Required bool    `json:"required"`
}

// FunctionExport describes one top-level callable binding.
type FunctionExport struct {
	Name       string        `json:"name"`
	Kind       string        `json:"kind"` // "function" | "generator"
	Params     []ParamExport `json:"params"`
	Arity      int           `json:"arity"`
	Required   int           `json:"required"` // number of required params
	ReturnType *string       `json:"return_type,omitempty"`
}

// MethodExport describes one class method.
type MethodExport struct {
	Name       string        `json:"name"`
	Params     []ParamExport `json:"params"`
	Arity      int           `json:"arity"`
	ReturnType *string       `json:"return_type,omitempty"`
}

// ClassExport describes one top-level class.
type ClassExport struct {
	Name    string         `json:"name"`
	Parent  *string        `json:"parent,omitempty"`
	Methods []MethodExport `json:"methods"`
}

// EnumExport describes one top-level enum.
type EnumExport struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

// ConstantExport describes one top-level constant binding.
type ConstantExport struct {
	Name  string `json:"name"`
	Value string `json:"value"` // rendered NvS source of the initializer
	Const bool   `json:"const"` // true for `const`, false for `let`
}

// Exports is the JSON document printed by `nvs exports`.
type Exports struct {
	Functions []FunctionExport `json:"functions"`
	Classes   []ClassExport    `json:"classes"`
	Enums     []EnumExport     `json:"enums"`
	Constants []ConstantExport `json:"constants"`
}

func renderType(t *ast.TypeAnnotation) *string {
	if t == nil {
		return nil
	}
	s := t.String()
	return &s
}

func exportParams(params []*ast.Identifier, defaults []ast.Expression, types []*ast.TypeAnnotation) []ParamExport {
	out := make([]ParamExport, 0, len(params))
	for i, p := range params {
		pe := ParamExport{Name: p.Value, Required: true}
		if i < len(defaults) && defaults[i] != nil {
			d := defaults[i].String()
			pe.Default = &d
			pe.Required = false
		}
		if i < len(types) {
			pe.Type = renderType(types[i])
		}
		out = append(out, pe)
	}
	return out
}

func countRequired(ps []ParamExport) int {
	n := 0
	for _, p := range ps {
		if p.Required {
			n++
		}
	}
	return n
}

func isGenerator(fn *ast.FunctionLiteral) bool {
	found := false
	astWalk(fn.Body, func(n ast.Node) {
		if _, ok := n.(*ast.YieldStatement); ok {
			found = true
		}
	})
	return found
}

// astWalk visits every node in a subtree (statements and expressions).
func astWalk(n ast.Node, visit func(ast.Node)) {
	if n == nil {
		return
	}
	visit(n)
	switch t := n.(type) {
	case *ast.BlockStatement:
		for _, s := range t.Statements {
			astWalk(s, visit)
		}
	case *ast.ExpressionStatement:
		astWalk(t.Expression, visit)
	case *ast.IfExpression:
		astWalk(t.Condition, visit)
		astWalk(t.Consequence, visit)
		if t.Alternative != nil {
			astWalk(t.Alternative, visit)
		}
	case *ast.WhileStatement:
		astWalk(t.Condition, visit)
		astWalk(t.Body, visit)
	case *ast.ForStatement:
		astWalk(t.Body, visit)
	case *ast.ForInStatement:
		astWalk(t.Iterable, visit)
		astWalk(t.Body, visit)
	case *ast.TryStatement:
		astWalk(t.Body, visit)
		if t.Catch != nil {
			astWalk(t.Catch, visit)
		}
		if t.Finally != nil {
			astWalk(t.Finally, visit)
		}
	case *ast.MatchExpression:
		astWalk(t.Value, visit)
		for _, a := range t.Arms {
			astWalk(a.Body, visit)
		}
	case *ast.FunctionLiteral:
		astWalk(t.Body, visit)
	}
}

// ExportSource parses NvS source and describes its public surface.
// Parse errors are returned; nothing is executed.
func ExportSource(src string) (*Exports, error) {
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("parse error: %s", strings.Join(errs, "; "))
	}
	exp := &Exports{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.LetStatement:
			// `fn name(...) {...}` desugars to let + FunctionLiteral.
			if fn, ok := s.Value.(*ast.FunctionLiteral); ok {
				params := exportParams(fn.Parameters, fn.Defaults, fn.ParamTypes)
				kind := "function"
				if isGenerator(fn) {
					kind = "generator"
				}
				exp.Functions = append(exp.Functions, FunctionExport{
					Name:       s.Name.Value,
					Kind:       kind,
					Params:     params,
					Arity:      len(params),
					Required:   countRequired(params),
					ReturnType: renderType(fn.ReturnType),
				})
			} else {
				exp.Constants = append(exp.Constants, ConstantExport{
					Name:  s.Name.Value,
					Value: s.Value.String(),
					Const: false,
				})
			}
		case *ast.ConstStatement:
			exp.Constants = append(exp.Constants, ConstantExport{
				Name:  s.Name.Value,
				Value: s.Value.String(),
				Const: true,
			})
		case *ast.ClassStatement:
			ce := ClassExport{Name: s.Name.Value}
			if s.Parent != nil {
				par := s.Parent.Value
				ce.Parent = &par
			}
			for _, m := range s.Methods {
				params := exportParams(m.Parameters, nil, m.ParamTypes)
				ce.Methods = append(ce.Methods, MethodExport{
					Name:       m.Name.Value,
					Params:     params,
					Arity:      len(params),
					ReturnType: renderType(m.ReturnType),
				})
			}
			exp.Classes = append(exp.Classes, ce)
		case *ast.EnumStatement:
			members := make([]string, 0, len(s.Members))
			for _, m := range s.Members {
				members = append(members, m.Value)
			}
			exp.Enums = append(exp.Enums, EnumExport{Name: s.Name.Value, Members: members})
		}
	}
	return exp, nil
}

// ExportSourceJSON renders the exports document as indented JSON.
func ExportSourceJSON(src string) (string, error) {
	exp, err := ExportSource(src)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}
