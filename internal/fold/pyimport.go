package fold

// Python-subset -> NvS importer (Wave 15, Part A).
//
// Documented subset (see docs/FOLDING.md for the full contract):
//   statements: =, += -= *= /= %= **= //=, def/return, if/elif/else,
//     while[/else], for x in ITER[/else], break, continue, pass,
//     print(...), assert e, raise e, bare call expressions
//   expressions: ints, floats, strings (+escapes), simple f-strings,
//     True/False/None, lists, dicts (str/int keys), indexing, slices
//     (no step), calls incl. keyword args, lambdas, single-level list
//     tuple assignment/swaps, chained assignment, boolean logic, and/or/not, comparisons,
//     is None / is not None, in / not in, arithmetic (+,-,*,/,%,**,//),
//     unary -/+
// Everything else -> *UnsupportedError naming the construct and line.

import (
	"fmt"
	"strings"
)

type pyParser struct {
	toks   []pyTok
	pos    int
	e      *emitter
	scopes []map[string]bool // defined names, innermost last
	tmpN   int
}

func importPython(src string) (string, error) {
	toks, err := lexPython(src)
	if err != nil {
		return "", err
	}
	p := &pyParser{toks: toks, e: &emitter{}, scopes: []map[string]bool{{}}}
	if err := p.parseProgram(); err != nil {
		return "", err
	}
	return p.e.String(), nil
}

// --- token helpers ---

func (p *pyParser) peek() pyTok { return p.toks[p.pos] }
func (p *pyParser) peek2() pyTok {
	if p.pos+1 < len(p.toks) {
		return p.toks[p.pos+1]
	}
	return pyTok{kind: pyEOF}
}
func (p *pyParser) next() pyTok {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}
func (p *pyParser) acceptOp(op string) bool {
	if t := p.peek(); t.kind == pyOp && t.text == op {
		p.next()
		return true
	}
	return false
}
func (p *pyParser) acceptName(name string) bool {
	if t := p.peek(); t.kind == pyName && t.text == name {
		p.next()
		return true
	}
	return false
}
func (p *pyParser) expectOp(op string) error {
	if !p.acceptOp(op) {
		t := p.peek()
		return &SyntaxError{Lang: "python", Line: t.line, Detail: fmt.Sprintf("expected %q, found %s", op, t)}
	}
	return nil
}
func (p *pyParser) unsupported(construct, detail string) *UnsupportedError {
	return &UnsupportedError{Lang: "python", Line: p.peek().line, Construct: construct, Detail: detail}
}

// --- scopes ---

func (p *pyParser) defined(name string) bool {
	for i := len(p.scopes) - 1; i >= 0; i-- {
		if p.scopes[i][name] {
			return true
		}
	}
	return false
}
func (p *pyParser) define(name string) { p.scopes[len(p.scopes)-1][name] = true }
func (p *pyParser) tmp() string {
	p.tmpN++
	return fmt.Sprintf("__py%d", p.tmpN)
}

// --- program ---

func (p *pyParser) parseProgram() error {
	for p.peek().kind != pyEOF {
		if p.peek().kind == pyNewline {
			p.next()
			continue
		}
		if err := p.parseStmt(); err != nil {
			return err
		}
	}
	return nil
}

func (p *pyParser) parseStmt() error {
	t := p.peek()
	if t.kind == pyName {
		switch t.text {
		case "def":
			return p.parseDef()
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile()
		case "for":
			return p.parseFor()
		case "return":
			return p.parseReturn()
		case "break":
			p.next()
			p.e.line("break")
			return p.endSimple()
		case "continue":
			p.next()
			p.e.line("continue")
			return p.endSimple()
		case "pass":
			p.next()
			return p.endSimple() // pass emits nothing
		case "raise":
			return p.parseRaise()
		case "assert":
			return p.parseAssert()
		case "import", "from":
			return p.unsupported("import statement", "module imports cannot be folded; inline the needed code instead")
		case "class":
			return p.unsupported("class definition", "Python classes are outside the importable subset")
		case "try":
			return p.unsupported("try/except", "exception handling blocks are outside the importable subset (raise/assert are supported)")
		case "with":
			return p.unsupported("with statement", "context managers are outside the importable subset")
		case "del":
			return p.unsupported("del statement", "deletion is outside the importable subset")
		case "global", "nonlocal":
			return p.unsupported(t.text+" declaration", "scope declarations are outside the importable subset")
		case "yield":
			return p.unsupported("yield", "generators are outside the importable subset")
		case "lambda":
			// lambda as a statement is meaningless; fall through to expr
		}
	}
	if t.kind == pyOp && t.text == "@" {
		return p.unsupported("decorator", "decorators are outside the importable subset")
	}
	return p.parseSimple()
}

// endSimple consumes the rest of a simple (single-line) statement:
// trailing "; stmt" chains and the final NEWLINE.
func (p *pyParser) endSimple() error {
	for p.acceptOp(";") {
		if err := p.parseSimpleNoSemi(); err != nil {
			return err
		}
	}
	if p.peek().kind != pyNewline && p.peek().kind != pyEOF {
		t := p.peek()
		return &SyntaxError{Lang: "python", Line: t.line, Detail: fmt.Sprintf("unexpected %s", t)}
	}
	if p.peek().kind == pyNewline {
		p.next()
	}
	return nil
}

func (p *pyParser) parseSimpleNoSemi() error {
	t := p.peek()
	if t.kind == pyName {
		switch t.text {
		case "return":
			return p.parseReturnNoEnd()
		case "break":
			p.next()
			p.e.line("break")
			return nil
		case "continue":
			p.next()
			p.e.line("continue")
			return nil
		case "pass":
			p.next()
			return nil
		case "raise":
			return p.parseRaiseNoEnd()
		case "assert":
			return p.parseAssertNoEnd()
		}
	}
	// Assignment? lookahead: NAME ( "[" ... "]" )* ("=" | augop)
	if p.peek().kind == pyName && p.isAssignAhead() {
		return p.parseAssignNoEnd()
	}
	// Tuple unpacking assignment: a, b = expr, expr
	if p.peek().kind == pyName && p.isTupleAssignAhead() {
		return p.parseTupleAssignNoEnd()
	}
	expr, err := p.parseExpr()
	if err != nil {
		return err
	}
	p.e.line(expr)
	return nil
}

// isAssignAhead scans for "=" or an augmented-assign op at bracket depth 0
// before the newline. A "," at depth 0 means tuple assignment (unsupported).
func (p *pyParser) isAssignAhead() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		switch t.kind {
		case pyNewline, pyEOF:
			return false
		case pyOp:
			switch t.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ",":
				if depth == 0 {
					return false
				}
			case "=", "+=", "-=", "*=", "/=", "%=", "**=", "//=":
				if depth == 0 {
					return true
				}
			}
		}
	}
	return false
}

// isTupleAssignAhead reports NAME (, NAME)* "=" ahead at depth 0.
func (p *pyParser) isTupleAssignAhead() bool {
	depth := 0
	sawComma := false
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		switch t.kind {
		case pyNewline, pyEOF:
			return false
		case pyOp:
			switch t.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ",":
				if depth == 0 {
					sawComma = true
				}
			case "=":
				if depth == 0 {
					return sawComma
				}
			case "+", "-", "*", "/", "%":
				// "+=" etc handled by isAssignAhead; a bare op here
				// means this is not a plain tuple assignment.
			}
		case pyName:
			if depth == 0 && sawComma {
				// target list must be bare names
			}
		}
	}
	return false
}

// parseTupleAssignNoEnd handles: a, b = e1, e2
func (p *pyParser) parseTupleAssignNoEnd() error {
	var targets []string
	for {
		if p.peek().kind != pyName {
			return p.unsupported("assignment target", "tuple unpacking supports only simple names")
		}
		targets = append(targets, p.next().text)
		if !p.acceptOp(",") {
			break
		}
	}
	if err := p.expectOp("="); err != nil {
		return err
	}
	var values []string
	for {
		v, err := p.parseExpr()
		if err != nil {
			return err
		}
		values = append(values, v)
		if !p.acceptOp(",") {
			break
		}
		if p.peek().kind == pyNewline || p.peek().kind == pyEOF {
			break // trailing comma
		}
	}
	if len(targets) != len(values) {
		return p.unsupported("unbalanced tuple assignment", "the number of targets must match the number of values")
	}
	// Evaluate all RHS first into temps (correct for swaps like a, b = b, a).
	tmps := make([]string, len(targets))
	for i, v := range values {
		tmps[i] = p.tmp()
		p.e.line("let " + tmps[i] + " = " + v)
	}
	for i, t := range targets {
		decl := ""
		if !p.defined(t) {
			decl = "let "
			p.define(t)
		}
		p.e.line(decl + t + " = " + tmps[i])
	}
	return nil
}

func (p *pyParser) parseSimple() error {
	if err := p.parseSimpleNoSemi(); err != nil {
		return err
	}
	return p.endSimple()
}

// --- statements ---

func (p *pyParser) parseAssignNoEnd() error {
	name := p.next().text // NAME
	// Optional subscripts on the LHS: x[i] = v (slices on the LHS are rejected)
	var lhs string = name
	for p.acceptOp("[") {
		if p.isSliceAhead() {
			return p.unsupported("slice assignment", "assigning to a slice is outside the importable subset")
		}
		idx, err := p.parseExpr()
		if err != nil {
			return err
		}
		if err := p.expectOp("]"); err != nil {
			return err
		}
		lhs += "[" + idx + "]"
	}
	op := p.next() // = or augop
	rhs, err := p.parseExpr()
	if err != nil {
		return err
	}
	decl := ""
	if lhs == name && !p.defined(name) {
		decl = "let "
		p.define(name)
	}
	switch op.text {
	case "=":
		p.e.line(decl + lhs + " = " + rhs)
	case "+=", "-=", "*=", "/=", "%=":
		binop := op.text[:1]
		p.e.line(lhs + " = " + lhs + " " + binop + " (" + rhs + ")")
	case "**=":
		p.e.line(lhs + " = pow(" + lhs + ", " + rhs + ")")
	case "//=":
		p.e.line(lhs + " = (" + lhs + " / " + rhs + ")")
	default:
		return p.unsupported("assignment operator "+op.text, "only =, +=, -=, *=, /=, %=, **=, //= are supported")
	}
	return nil
}

func (p *pyParser) parseReturnNoEnd() error {
	p.next() // return
	if p.peek().kind == pyNewline || p.peek().kind == pyOp && p.peek().text == ";" {
		p.e.line("return")
		return nil
	}
	expr, err := p.parseExpr()
	if err != nil {
		return err
	}
	if strings.HasPrefix(expr, "__tuple") {
		return p.unsupported("returning multiple values", "return a list instead of a tuple")
	}
	p.e.line("return " + expr)
	return nil
}

func (p *pyParser) parseReturn() error {
	if err := p.parseReturnNoEnd(); err != nil {
		return err
	}
	return p.endSimple()
}

func (p *pyParser) parseRaiseNoEnd() error {
	p.next() // raise
	expr, err := p.parseExpr()
	if err != nil {
		return err
	}
	// raise ValueError("msg") -> throw("msg"); raise "msg" -> throw("msg")
	msg := expr
	if strings.HasPrefix(expr, "__call:") {
		// encoded as __call:Name(arg) by parseCall for special handling
		rest := strings.TrimPrefix(expr, "__call:")
		if i := strings.Index(rest, "("); i > 0 {
			name := rest[:i]
			args := rest[i+1 : len(rest)-1]
			if args != "" && !strings.Contains(args, ",") &&
				strings.HasPrefix(strings.TrimSpace(args), "\"") {
				msg = strings.TrimSpace(args)
			} else {
				return p.unsupported("raise "+name+"(...)", "only raise with a single string message is supported")
			}
		}
	}
	p.e.line("throw(" + msg + ")")
	return nil
}

func (p *pyParser) parseRaise() error {
	if err := p.parseRaiseNoEnd(); err != nil {
		return err
	}
	return p.endSimple()
}

func (p *pyParser) parseAssertNoEnd() error {
	p.next() // assert
	cond, err := p.parseExpr()
	if err != nil {
		return err
	}
	if p.acceptOp(",") {
		return p.unsupported("assert with message", "only bare assert <expr> is supported")
	}
	p.e.line("assert(" + cond + ")")
	return nil
}

func (p *pyParser) parseAssert() error {
	if err := p.parseAssertNoEnd(); err != nil {
		return err
	}
	return p.endSimple()
}

// parseSuite parses ": NEWLINE INDENT stmts DEDENT".
func (p *pyParser) parseSuite() error {
	if err := p.expectOp(":"); err != nil {
		return err
	}
	if p.peek().kind != pyNewline {
		return &SyntaxError{Lang: "python", Line: p.peek().line,
			Detail: "expected a newline and indented block after ':' (single-line suites are outside the subset)"}
	}
	p.next() // NEWLINE
	if p.peek().kind != pyIndent {
		return &SyntaxError{Lang: "python", Line: p.peek().line, Detail: "expected an indented block"}
	}
	p.next() // INDENT
	for p.peek().kind != pyDedent && p.peek().kind != pyEOF {
		if p.peek().kind == pyNewline {
			p.next()
			continue
		}
		if err := p.parseStmt(); err != nil {
			return err
		}
	}
	if p.peek().kind == pyEOF {
		return &SyntaxError{Lang: "python", Line: p.peek().line, Detail: "unexpected end of file inside indented block"}
	}
	p.next() // DEDENT
	return nil
}

func (p *pyParser) parseDef() error {
	p.next() // def
	if p.peek().kind != pyName {
		return &SyntaxError{Lang: "python", Line: p.peek().line, Detail: "expected function name after 'def'"}
	}
	name := p.next().text
	if err := p.expectOp("("); err != nil {
		return err
	}
	var params []string
	paramNames := []string{}
	if !p.acceptOp(")") {
		for {
			if p.acceptOp("*") {
				return p.unsupported("*args", "variadic *args is outside the importable subset")
			}
			if p.peek().kind != pyName {
				return &SyntaxError{Lang: "python", Line: p.peek().line, Detail: "expected parameter name"}
			}
			pname := p.next().text
			// Optional annotation ": expr" — parsed and discarded.
			if p.acceptOp(":") {
				if _, err := p.parseExpr(); err != nil {
					return err
				}
			}
			param := pname
			if p.acceptOp("=") {
				dflt, err := p.parseExpr()
				if err != nil {
					return err
				}
				param = pname + "=" + dflt
			}
			params = append(params, param)
			paramNames = append(paramNames, pname)
			if p.acceptOp(",") {
				if p.peek().kind == pyOp && p.peek().text == ")" {
					break // trailing comma
				}
				continue
			}
			break
		}
		if err := p.expectOp(")"); err != nil {
			return err
		}
	}
	// Optional return annotation "-> expr" — parsed and discarded.
	if p.acceptOp("-") && p.acceptOp(">") {
		// (lexer emits "->" as one op; this branch is defensive)
		if _, err := p.parseExpr(); err != nil {
			return err
		}
	} else if p.peek().kind == pyOp && p.peek().text == "->" {
		p.next()
		if _, err := p.parseExpr(); err != nil {
			return err
		}
	}
	p.e.open("fn " + name + "(" + strings.Join(params, ", ") + ")")
	// New scope seeded with params.
	p.scopes = append(p.scopes, map[string]bool{})
	for _, pn := range paramNames {
		p.define(pn)
	}
	err := p.parseSuite()
	p.scopes = p.scopes[:len(p.scopes)-1]
	if err != nil {
		return err
	}
	p.e.close()
	p.define(name)
	return nil
}

func (p *pyParser) parseIf() error {
	p.next() // if
	cond, err := p.parseExpr()
	if err != nil {
		return err
	}
	p.e.open("if (" + cond + ")")
	if err := p.parseSuite(); err != nil {
		return err
	}
	for p.acceptName("elif") {
		cond, err := p.parseExpr()
		if err != nil {
			return err
		}
		p.e.closeElse("else if (" + cond + ")")
		if err := p.parseSuite(); err != nil {
			return err
		}
	}
	if p.acceptName("else") {
		p.e.closeElse("else")
		if err := p.parseSuite(); err != nil {
			return err
		}
	}
	p.e.close()
	return nil
}

func (p *pyParser) parseWhile() error {
	p.next() // while
	cond, err := p.parseExpr()
	if err != nil {
		return err
	}
	p.e.open("while (" + cond + ")")
	if err := p.parseSuite(); err != nil {
		return err
	}
	if p.acceptName("else") {
		p.e.closeElse("else")
		if err := p.parseSuite(); err != nil {
			return err
		}
	}
	p.e.close()
	return nil
}

func (p *pyParser) parseFor() error {
	p.next() // for
	// Target: name or name, name (tuple unpack, only for enumerate())
	var targets []string
	for {
		if p.peek().kind != pyName {
			return p.unsupported("for-loop target", "only simple names (or a, b unpacking with enumerate()) are supported")
		}
		targets = append(targets, p.next().text)
		if !p.acceptOp(",") {
			break
		}
	}
	if !p.acceptName("in") {
		return &SyntaxError{Lang: "python", Line: p.peek().line, Detail: "expected 'in' in for loop"}
	}
	// Special case: for i, v in enumerate(xs):
	if len(targets) == 2 && p.peek().kind == pyName && p.peek().text == "enumerate" && p.peek2().kind == pyOp && p.peek2().text == "(" {
		return p.parseForEnumerate(targets[0], targets[1])
	}
	if len(targets) != 1 {
		return p.unsupported("tuple unpacking in for loop", "only 'for i, v in enumerate(xs)' unpacking is supported")
	}
	iter, err := p.parseExpr()
	if err != nil {
		return err
	}
	p.e.open("for (" + targets[0] + " in " + iter + ")")
	p.define(targets[0])
	if err := p.parseSuite(); err != nil {
		return err
	}
	if p.acceptName("else") {
		p.e.closeElse("else")
		if err := p.parseSuite(); err != nil {
			return err
		}
	}
	p.e.close()
	return nil
}

func (p *pyParser) parseForEnumerate(t1, t2 string) error {
	p.next() // enumerate
	if err := p.expectOp("("); err != nil {
		return err
	}
	xs, err := p.parseExpr()
	if err != nil {
		return err
	}
	if err := p.expectOp(")"); err != nil {
		return err
	}
	tmp := p.tmp()
	p.e.open("for (" + tmp + " in enumerate(" + xs + "))")
	p.e.line("let " + t1 + " = " + tmp + "[0]")
	p.e.line("let " + t2 + " = " + tmp + "[1]")
	p.define(t1)
	p.define(t2)
	if err := p.parseSuite(); err != nil {
		return err
	}
	p.e.close()
	return nil
}

// --- expressions ---

func (p *pyParser) parseExpr() (string, error) { return p.parseOr() }

func (p *pyParser) parseOr() (string, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.acceptName("or") {
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		left = "(" + left + " or " + right + ")"
	}
	return left, nil
}

func (p *pyParser) parseAnd() (string, error) {
	left, err := p.parseNot()
	if err != nil {
		return "", err
	}
	for p.acceptName("and") {
		right, err := p.parseNot()
		if err != nil {
			return "", err
		}
		left = "(" + left + " and " + right + ")"
	}
	return left, nil
}

func (p *pyParser) parseNot() (string, error) {
	if p.acceptName("not") {
		// "not in" is handled at comparison level; bare "not x"
		e, err := p.parseNot()
		if err != nil {
			return "", err
		}
		return "(!(" + e + "))", nil
	}
	return p.parseComp()
}

func (p *pyParser) parseComp() (string, error) {
	left, err := p.parseArith()
	if err != nil {
		return "", err
	}
	t := p.peek()
	negate := false
	if t.kind == pyName && t.text == "not" {
		// must be "not in"
		if p.peek2().kind == pyName && p.peek2().text == "in" {
			p.next()
			p.next()
			negate = true
		} else {
			return left, nil // "not" belongs to an outer level
		}
	} else if t.kind == pyName && t.text == "in" {
		p.next()
	} else if t.kind == pyName && t.text == "is" {
		p.next()
		notIs := p.acceptName("not")
		if !p.acceptName("None") {
			return "", p.unsupported("'is' comparison", "only 'is None' / 'is not None' are supported")
		}
		op := "=="
		if notIs {
			op = "!="
		}
		return "(" + left + " " + op + " null)", p.checkNoChained()
	} else if t.kind == pyOp && (t.text == "<" || t.text == ">" || t.text == "<=" || t.text == ">=" || t.text == "==" || t.text == "!=") {
		op := t.text
		p.next()
		right, err := p.parseArith()
		if err != nil {
			return "", err
		}
		return "(" + left + " " + op + " " + right + ")", p.checkNoChained()
	} else {
		return left, nil
	}
	// "in" / "not in"
	right, err := p.parseArith()
	if err != nil {
		return "", err
	}
	// index_of works on both strings and arrays in NvS; for a hash
	// literal, membership means key membership.
	target := right
	if strings.HasPrefix(strings.TrimSpace(right), "{") {
		target = "keys(" + right + ")"
	}
	res := "(index_of(" + target + ", " + left + ") != -1)"
	if negate {
		res = "(!(" + res + "))"
	}
	return res, p.checkNoChained()
}

func (p *pyParser) checkNoChained() error {
	t := p.peek()
	if t.kind == pyOp && (t.text == "<" || t.text == ">" || t.text == "<=" || t.text == ">=" || t.text == "==" || t.text == "!=") {
		return p.unsupported("chained comparison", "rewrite as explicit and-comparisons")
	}
	return nil
}

func (p *pyParser) parseArith() (string, error) {
	left, err := p.parseTerm()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind == pyOp && (t.text == "+" || t.text == "-") {
			p.next()
			right, err := p.parseTerm()
			if err != nil {
				return "", err
			}
			if t.text == "+" {
				if add, ok := pyListAdd(left, right); ok {
					left = add
					continue
				}
			}
			left = "(" + left + " " + t.text + " " + right + ")"
			continue
		}
		return left, nil
	}
}

// pyListAdd folds Python list concatenation into NvS spread syntax when at
// least one operand is syntactically a list literal: [1]+[2] -> [1, 2],
// xs+[1] -> [...xs, 1], [1]+xs -> [1, ...xs].
func pyListAdd(l, r string) (string, bool) {
	lok := isListLiteral(l)
	rok := isListLiteral(r)
	inner := func(s string) string { return s[1 : len(s)-1] }
	switch {
	case lok && rok:
		li, ri := inner(l), inner(r)
		switch {
		case li == "" && ri == "":
			return "[]", true
		case li == "":
			return "[" + ri + "]", true
		case ri == "":
			return "[" + li + "]", true
		}
		return "[" + li + ", " + ri + "]", true
	case rok:
		ri := inner(r)
		if ri == "" {
			return "[..." + l + "]", true
		}
		return "[..." + l + ", " + ri + "]", true
	case lok:
		li := inner(l)
		if li == "" {
			return "[..." + r + "]", true
		}
		return "[" + li + ", ..." + r + "]", true
	}
	return "", false
}

// isListLiteral reports whether s is a single top-level [...] literal,
// string-aware so ["a]b"] and [x][0] are handled correctly.
func isListLiteral(s string) bool {
	if len(s) < 2 || s[0] != '[' {
		return false
	}
	depth := 0
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i == len(s)-1
			}
			if depth < 0 {
				return false
			}
		}
	}
	return false
}

// isStrLiteral reports whether s is a double-quoted string literal.
func isStrLiteral(s string) bool {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return false
	}
	esc := false
	for i := 1; i < len(s)-1; i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
		}
	}
	return true
}

func (p *pyParser) parseTerm() (string, error) {
	left, err := p.parseFactor()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind == pyOp && (t.text == "*" || t.text == "/" || t.text == "%" || t.text == "//") {
			p.next()
			right, err := p.parseFactor()
			if err != nil {
				return "", err
			}
			op := t.text
			if op == "//" {
				// Documented divergence: NvS truncates toward zero,
				// Python floors (differ only for negative operands).
				op = "/"
			}
			if op == "*" {
				// "ab" * 3 -> repeat("ab", 3); 3 * "ab" likewise.
				// [1] * 3 has no NvS counterpart -> loud error.
				if isStrLiteral(left) {
					left = "repeat(" + left + ", " + right + ")"
					continue
				}
				if isStrLiteral(right) {
					left = "repeat(" + right + ", " + left + ")"
					continue
				}
				if isListLiteral(left) || isListLiteral(right) {
					return "", p.unsupported("list repetition", "repeating a list with * has no NvS counterpart; use a loop or append")
				}
			}
			left = "(" + left + " " + op + " " + right + ")"
			continue
		}
		return left, nil
	}
}

func (p *pyParser) parseFactor() (string, error) {
	if t := p.peek(); t.kind == pyOp && (t.text == "-" || t.text == "+") {
		p.next()
		e, err := p.parseFactor()
		if err != nil {
			return "", err
		}
		if t.text == "-" {
			return "(-(" + e + "))", nil
		}
		return e, nil
	}
	return p.parsePower()
}

func (p *pyParser) parsePower() (string, error) {
	base, err := p.parsePostfix()
	if err != nil {
		return "", err
	}
	if p.acceptOp("**") {
		exp, err := p.parseFactor() // right-associative, unary binds looser
		if err != nil {
			return "", err
		}
		return "pow(" + base + ", " + exp + ")", nil
	}
	return base, nil
}

func (p *pyParser) parsePostfix() (string, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return "", err
	}
	for {
		switch {
		case p.acceptOp("."):
			t := p.peek()
			if t.kind != pyName {
				return "", &SyntaxError{Lang: "python", Line: t.line, Detail: "expected attribute name after '.'"}
			}
			p.next()
			if p.peek().kind == pyOp && p.peek().text == "(" {
				return "", p.unsupported("method call ."+t.text+"()", "method calls are outside the importable subset")
			}
			return "", p.unsupported("attribute access ."+t.text, "attribute access is outside the importable subset")
		case p.peek().kind == pyOp && p.peek().text == "[":
			sub, err := p.parseSubscript()
			if err != nil {
				return "", err
			}
			if strings.HasPrefix(sub, "\x00SLICE\x00(") {
				inner := strings.TrimSuffix(strings.TrimPrefix(sub, "\x00SLICE\x00("), ")")
				inner = strings.ReplaceAll(inner, "\x00LEN\x00", "len("+node+")")
				node = "slice(" + node + ", " + inner + ")"
			} else {
				node = node + sub
			}
		case p.peek().kind == pyOp && p.peek().text == "(":
			node, err = p.parseCall(node)
			if err != nil {
				return "", err
			}
		default:
			return node, nil
		}
	}
}

// parseSubscript parses "[...]" after the opening bracket was peeked.
// Returns the NvS suffix: "[i]" or slice(...) wrapping handled by caller.
func (p *pyParser) parseSubscript() (string, error) {
	p.next() // [
	// Slice detection: look for ":" at depth 0 before "]".
	if p.isSliceAhead() {
		return p.parseSlice()
	}
	idx, err := p.parseExpr()
	if err != nil {
		return "", err
	}
	if err := p.expectOp("]"); err != nil {
		return "", err
	}
	return "[" + idx + "]", nil
}

func (p *pyParser) isSliceAhead() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		if t.kind == pyOp {
			switch t.text {
			case "[", "(", "{":
				depth++
			case "]":
				if depth == 0 {
					return false
				}
				depth--
			case ")", "}":
				depth--
			case ":":
				if depth == 0 {
					return true
				}
			}
		}
		if t.kind == pyNewline || t.kind == pyEOF {
			return false
		}
	}
	return false
}

// parseSlice parses the inside of a[i:j:k] (opening "[" consumed).
// Returns a suffix expression using the slice() builtin; the caller
// wraps it around the base object.
func (p *pyParser) parseSlice() (string, error) {
	// We return a marker "__slice:lo:hi" and let the caller combine;
	// simpler: build "slice(BASE, lo, hi)" — but base isn't known here.
	// So return a template the caller fills: "\x00BASE\x00" placeholder.
	var lo, hi string
	var err error
	if p.peek().kind == pyOp && (p.peek().text == ":" || p.peek().text == "]") {
		lo = "0"
	} else {
		lo, err = p.parseExpr()
		if err != nil {
			return "", err
		}
	}
	if err := p.expectOp(":"); err != nil {
		return "", err
	}
	if p.peek().kind == pyOp && (p.peek().text == ":" || p.peek().text == "]") {
		hi = "" // filled by caller as len(base)
	} else {
		hi, err = p.parseExpr()
		if err != nil {
			return "", err
		}
	}
	if p.acceptOp(":") {
		return "", p.unsupported("slice with step", "strided slices are outside the importable subset")
	}
	if err := p.expectOp("]"); err != nil {
		return "", err
	}
	if hi == "" {
		hi = "\x00LEN\x00"
	}
	return "\x00SLICE\x00(" + lo + ", " + hi + ")", nil
}

func (p *pyParser) parsePrimary() (string, error) {
	t := p.peek()
	switch t.kind {
	case pyInt, pyFloat:
		p.next()
		return t.text, nil
	case pyStr:
		p.next()
		if t.fstr {
			return p.convertFString(t.val, t.line)
		}
		return nvsQuote(t.val), nil
	case pyName:
		switch t.text {
		case "True":
			p.next()
			return "true", nil
		case "False":
			p.next()
			return "false", nil
		case "None":
			p.next()
			return "null", nil
		case "lambda":
			return p.parseLambda()
		}
		p.next()
		return t.text, nil
	case pyOp:
		switch t.text {
		case "(":
			p.next()
			if p.acceptOp(")") {
				return "", p.unsupported("empty tuple ()", "tuples are outside the importable subset")
			}
			e, err := p.parseExpr()
			if err != nil {
				return "", err
			}
			if p.acceptOp(",") {
				return "", p.unsupported("tuple literal", "tuples are outside the importable subset; use a list")
			}
			if err := p.expectOp(")"); err != nil {
				return "", err
			}
			return "(" + e + ")", nil
		case "[":
			return p.parseListOrCompr()
		case "{":
			return p.parseDict()
		}
	}
	return "", &SyntaxError{Lang: "python", Line: t.line, Detail: fmt.Sprintf("unexpected %s in expression", t)}
}

func (p *pyParser) parseListOrCompr() (string, error) {
	p.next() // [
	if p.acceptOp("]") {
		return "[]", nil
	}
	first, err := p.parseExpr()
	if err != nil {
		return "", err
	}
	// Comprehension?
	if p.peek().kind == pyName && p.peek().text == "for" {
		return p.parseComprehension(first)
	}
	items := []string{first}
	for p.acceptOp(",") {
		if p.peek().kind == pyOp && p.peek().text == "]" {
			break // trailing comma
		}
		it, err := p.parseExpr()
		if err != nil {
			return "", err
		}
		items = append(items, it)
	}
	if err := p.expectOp("]"); err != nil {
		return "", err
	}
	return "[" + strings.Join(items, ", ") + "]", nil
}

// [EXPR for x in ITER] / [EXPR for x in ITER if COND]
//
// Comprehensions are OUTSIDE the importable subset: they are rejected loudly
// (never silently miscompiled) with the construct name and line number.
func (p *pyParser) parseComprehension(elt string) (string, error) {
	_ = elt
	return "", p.unsupported("list comprehension", "comprehensions are outside the importable subset; use an explicit for loop with append")
}

func (p *pyParser) parseDict() (string, error) {
	p.next() // {
	if p.acceptOp("}") {
		return "{}", nil
	}
	var pairs []string
	for {
		k, err := p.parseExpr()
		if err != nil {
			return "", err
		}
		if p.peek().kind == pyName && p.peek().text == "for" {
			return "", p.unsupported("dict/set comprehension", "comprehensions over dicts/sets are outside the importable subset")
		}
		if err := p.expectOp(":"); err != nil {
			return "", &SyntaxError{Lang: "python", Line: p.peek().line,
				Detail: "set literals are outside the importable subset (use a list)"}
		}
		v, err := p.parseExpr()
		if err != nil {
			return "", err
		}
		pairs = append(pairs, k+": "+v)
		if p.peek().kind == pyName && p.peek().text == "for" {
			return "", p.unsupported("dict comprehension", "comprehensions are outside the importable subset; use an explicit for loop")
		}
		if !p.acceptOp(",") {
			break
		}
		if p.peek().kind == pyOp && p.peek().text == "}" {
			break
		}
	}
	if err := p.expectOp("}"); err != nil {
		return "", err
	}
	return "{" + strings.Join(pairs, ", ") + "}", nil
}

func (p *pyParser) parseLambda() (string, error) {
	p.next() // lambda
	var params []string
	for {
		if p.peek().kind != pyName {
			return "", &SyntaxError{Lang: "python", Line: p.peek().line, Detail: "expected lambda parameter"}
		}
		params = append(params, p.next().text)
		if !p.acceptOp(",") {
			break
		}
	}
	if err := p.expectOp(":"); err != nil {
		return "", err
	}
	body, err := p.parseExpr()
	if err != nil {
		return "", err
	}
	return "fn(" + strings.Join(params, ", ") + ") { return " + body + " }", nil
}

// parseCall parses "(args)" after func was parsed. func is the NvS callee expr.
func (p *pyParser) parseCall(funcExpr string) (string, error) {
	p.next() // (
	var args []string
	var kwargs [][2]string
	if !p.acceptOp(")") {
		for {
			if p.peek().kind == pyOp && (p.peek().text == "*" || p.peek().text == "**") {
				return "", p.unsupported("star-args in calls", "*args/**kwargs are outside the importable subset")
			}
			// keyword arg? NAME = ...
			if p.peek().kind == pyName && p.peek2().kind == pyOp && p.peek2().text == "=" {
				kname := p.next().text
				p.next() // =
				v, err := p.parseExpr()
				if err != nil {
					return "", err
				}
				kwargs = append(kwargs, [2]string{kname, v})
			} else {
				a, err := p.parseExpr()
				if err != nil {
					return "", err
				}
				args = append(args, a)
			}
			if !p.acceptOp(",") {
				break
			}
			if p.peek().kind == pyOp && p.peek().text == ")" {
				break
			}
		}
		if err := p.expectOp(")"); err != nil {
			return "", err
		}
	}
	for _, kw := range kwargs {
		args = append(args, kw[0]+"="+kw[1])
	}
	argStr := strings.Join(args, ", ")

	// Builtin mappings.
	switch funcExpr {
	case "print":
		switch len(args) {
		case 0:
			return `print("")`, nil
		case 1:
			if len(kwargs) > 0 {
				return "", p.unsupported("print() with keyword args", "only print(values) is supported")
			}
			return "print(" + args[0] + ")", nil
		default:
			if len(kwargs) > 0 {
				return "", p.unsupported("print() with keyword args", "only print(values) is supported")
			}
			strs := make([]string, len(args))
			for i, a := range args {
				strs[i] = "str(" + a + ")"
			}
			return `print(join([` + strings.Join(strs, ", ") + `], " "))`, nil
		}
	case "sorted":
		if len(args) == 1 && len(kwargs) == 0 {
			return "sort(" + args[0] + ")", nil
		}
		return "", p.unsupported("sorted() with key/reverse", "only sorted(list) is supported")
	case "range", "len", "str", "int", "float", "abs", "min", "max", "sum", "enumerate", "all", "any", "round":
		return funcExpr + "(" + argStr + ")", nil
	case "repr", "type", "zip", "map", "filter", "reversed", "open", "input", "eval", "exec", "getattr", "setattr", "hasattr", "isinstance", "super", "chr", "ord", "hex", "oct", "bin", "divmod", "pow":
		return "", p.unsupported(funcExpr+"()", "this builtin is outside the importable subset")
	}
	// __call:Name(...) marker lets parseRaise detect raise ValueError("msg").
	if isExcName(funcExpr) {
		return "__call:" + funcExpr + "(" + argStr + ")", nil
	}
	return funcExpr + "(" + argStr + ")", nil
}

func isExcName(name string) bool {
	switch name {
	case "ValueError", "TypeError", "RuntimeError", "Exception", "KeyError", "IndexError", "AttributeError", "NotImplementedError":
		return true
	}
	return false
}

// convertFString converts a raw f-string body (escapes already processed,
// braces intact) to an NvS interpolated string.
func (p *pyParser) convertFString(raw string, line int) (string, error) {
	var sb strings.Builder
	sb.WriteString("\"")
	i := 0
	for i < len(raw) {
		c := raw[i]
		switch c {
		case '{':
			if i+1 < len(raw) && raw[i+1] == '{' {
				sb.WriteString("{")
				i += 2
				continue
			}
			j, err := matchBrace(raw, i)
			if err != nil {
				return "", &SyntaxError{Lang: "python", Line: line, Detail: "unmatched '{' in f-string"}
			}
			inner := raw[i+1 : j]
			if hasFormatSpec(inner) {
				return "", &UnsupportedError{Lang: "python", Line: line, Construct: "f-string format spec",
					Detail: "only plain {expr} holes are supported (no :spec, !r/!s, or =)"}
			}
			sub, err := p.parseFStrExpr(inner, line)
			if err != nil {
				return "", err
			}
			sb.WriteString("${" + sub + "}")
			i = j + 1
		case '}':
			if i+1 < len(raw) && raw[i+1] == '}' {
				sb.WriteString("}")
				i += 2
				continue
			}
			return "", &SyntaxError{Lang: "python", Line: line, Detail: "unmatched '}' in f-string"}
		case '"':
			sb.WriteString("\\\"")
			i++
		case '\\':
			sb.WriteString("\\\\")
			i++
		case '\n':
			sb.WriteString("\\n")
			i++
		case '$':
			// Avoid accidental interpolation if user text has "${".
			if i+1 < len(raw) && raw[i+1] == '{' {
				sb.WriteString("\\$")
			} else {
				sb.WriteByte(c)
			}
			i++
		default:
			sb.WriteByte(c)
			i++
		}
	}
	sb.WriteString("\"")
	return sb.String(), nil
}

func hasFormatSpec(inner string) bool {
	depth := 0
	var quote byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			depth--
		case ':', '!', '=':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func matchBrace(s string, i int) (int, error) {
	depth := 0
	var quote byte
	for j := i; j < len(s); j++ {
		c := s[j]
		if quote != 0 {
			if c == '\\' {
				j++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j, nil
			}
		}
	}
	return 0, fmt.Errorf("unmatched")
}

// parseFStrExpr parses an f-string hole expression by re-lexing it.
func (p *pyParser) parseFStrExpr(inner string, line int) (string, error) {
	toks, dd, err := lexPyLine(inner, line)
	if err != nil {
		return "", err
	}
	if dd != 0 {
		return "", &SyntaxError{Lang: "python", Line: line, Detail: "unbalanced brackets in f-string expression"}
	}
	toks = append(toks, pyTok{kind: pyEOF, line: line})
	sub := &pyParser{toks: toks, e: &emitter{}, scopes: p.scopes}
	return sub.parseExpr()
}

// nvsQuote renders a decoded Go string as an NvS double-quoted literal.
func nvsQuote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\t':
			sb.WriteString("\\t")
		case '\r':
			sb.WriteString("\\r")
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
