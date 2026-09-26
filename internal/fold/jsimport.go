package fold

// JavaScript-subset -> NvS importer (Wave 15, Part A).
//
// Documented subset (see docs/FOLDING.md for the full contract):
//   var/let/const (var -> let), destructuring let [a,b]/let {x,y},
//   function decls, anonymous function exprs, arrow functions,
//   if/else, while, C-style for, for-of, break, continue, return,
//   throw, console.log(...), template literals (simple ${expr}),
//   ===/!== -> ==/!=, &&/|| -> and/or, ?? passthrough, ?. passthrough,
//   arrays, objects (incl. {...spread}), spread in calls/arrays,
//   .length -> len(), .push()/.pop(), Math.max/min/abs/floor/ceil/
//   round/sqrt/pow/random, new-less calls, comments.
// Everything else -> *UnsupportedError naming the construct and line.

import (
	"fmt"
	"strings"
)

type jsParser struct {
	toks   []jsTok
	pos    int
	e      *emitter
	scopes []map[string]bool
}

func importJS(src string) (string, error) {
	toks, err := lexJS(src)
	if err != nil {
		return "", err
	}
	p := &jsParser{toks: toks, e: &emitter{}, scopes: []map[string]bool{{}}}
	if err := p.parseProgram(); err != nil {
		return "", err
	}
	return p.e.String(), nil
}

// --- token helpers ---

func (p *jsParser) peek() jsTok { return p.toks[p.pos] }
func (p *jsParser) peek2() jsTok {
	if p.pos+1 < len(p.toks) {
		return p.toks[p.pos+1]
	}
	return jsTok{kind: jsEOF}
}
func (p *jsParser) next() jsTok {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}
func (p *jsParser) acceptOp(op string) bool {
	if t := p.peek(); t.kind == jsOp && t.text == op {
		p.next()
		return true
	}
	return false
}
func (p *jsParser) acceptName(name string) bool {
	if t := p.peek(); t.kind == jsName && t.text == name {
		p.next()
		return true
	}
	return false
}
func (p *jsParser) expectOp(op string) error {
	if !p.acceptOp(op) {
		t := p.peek()
		return &SyntaxError{Lang: "js", Line: t.line, Detail: fmt.Sprintf("expected %q, found %s", op, t)}
	}
	return nil
}
func (p *jsParser) unsupported(construct, detail string) *UnsupportedError {
	return &UnsupportedError{Lang: "js", Line: p.peek().line, Construct: construct, Detail: detail}
}
func (p *jsParser) semi() {
	p.acceptOp(";")
}

// --- scopes ---

func (p *jsParser) defined(name string) bool {
	for i := len(p.scopes) - 1; i >= 0; i-- {
		if p.scopes[i][name] {
			return true
		}
	}
	return false
}
func (p *jsParser) define(name string) { p.scopes[len(p.scopes)-1][name] = true }

// --- program ---

func (p *jsParser) parseProgram() error {
	for p.peek().kind != jsEOF {
		if err := p.parseStmt(); err != nil {
			return err
		}
	}
	return nil
}

func (p *jsParser) parseStmt() error {
	t := p.peek()
	if t.kind == jsName {
		// async (function/async arrow) must never silently degrade to sync.
		if t.text == "async" {
			return p.unsupported("async function", "async/await is outside the importable subset")
		}
		switch t.text {
		case "var", "let", "const":
			return p.parseVarDecl()
		case "function":
			return p.parseFuncDecl()
		case "if":
			return p.parseIf()
		case "while":
			return p.parseWhile()
		case "do":
			return p.unsupported("do-while", "do-while loops are outside the importable subset")
		case "for":
			return p.parseFor()
		case "switch":
			return p.unsupported("switch", "switch statements are outside the importable subset (NvS match differs in fallthrough semantics)")
		case "return":
			p.next()
			if p.peek().kind == jsOp && (p.peek().text == ";" || p.peek().text == "}") || p.peek().kind == jsEOF {
				p.e.line("return")
			} else {
				e, err := p.parseExpr()
				if err != nil {
					return err
				}
				p.e.line("return " + e)
			}
			p.semi()
			return nil
		case "break":
			p.next()
			// labeled break: break label; — NvS has labels; support plain only
			if p.peek().kind == jsName {
				return p.unsupported("labeled break", "labeled break/continue are outside the importable subset")
			}
			p.e.line("break")
			p.semi()
			return nil
		case "continue":
			p.next()
			if p.peek().kind == jsName {
				return p.unsupported("labeled continue", "labeled break/continue are outside the importable subset")
			}
			p.e.line("continue")
			p.semi()
			return nil
		case "throw":
			return p.unsupported("throw statement", "exceptions are outside the importable subset")
		case "try":
			return p.unsupported("try/catch", "exception handling blocks are outside the importable subset")
		case "class":
			return p.unsupported("class", "JS classes are outside the importable subset")
		case "import", "export":
			return p.unsupported(t.text+" statement", "module imports/exports cannot be folded; inline the needed code instead")
		case "debugger":
			p.next()
			p.semi()
			return nil
		}
	}
	if t.kind == jsOp && t.text == "{" {
		return p.parseBlock()
	}
	// Expression statement.
	e, err := p.parseExpr()
	if err != nil {
		return err
	}
	p.e.line(e)
	p.semi()
	return nil
}

func (p *jsParser) parseBlock() error {
	if err := p.expectOp("{"); err != nil {
		return err
	}
	p.e.line("{")
	p.e.ind++
	p.scopes = append(p.scopes, map[string]bool{})
	for p.peek().kind != jsEOF && !(p.peek().kind == jsOp && p.peek().text == "}") {
		if err := p.parseStmt(); err != nil {
			return err
		}
	}
	p.scopes = p.scopes[:len(p.scopes)-1]
	if err := p.expectOp("}"); err != nil {
		return err
	}
	p.e.close()
	return nil
}

// parseBlockBody parses "{" stmts "}" assuming "{" already consumed,
// emitting with the already-opened emitter level.
func (p *jsParser) parseBlockBody() error {
	p.scopes = append(p.scopes, map[string]bool{})
	for p.peek().kind != jsEOF && !(p.peek().kind == jsOp && p.peek().text == "}") {
		if err := p.parseStmt(); err != nil {
			return err
		}
	}
	p.scopes = p.scopes[:len(p.scopes)-1]
	return p.expectOp("}")
}

func (p *jsParser) parseVarDecl() error {
	kw := p.next().text // var|let|const
	nvsKw := kw
	if kw == "var" {
		nvsKw = "let"
	}
	for {
		if p.peek().kind == jsOp && p.peek().text == "{" {
			if err := p.parseDestructureObj(nvsKw); err != nil {
				return err
			}
		} else if p.peek().kind == jsOp && p.peek().text == "[" {
			if err := p.parseDestructureArr(nvsKw); err != nil {
				return err
			}
		} else {
			if p.peek().kind != jsName {
				return &SyntaxError{Lang: "js", Line: p.peek().line, Detail: "expected variable name"}
			}
			name := p.next().text
			decl := nvsKw + " " + name
			if p.acceptOp("=") {
				e, err := p.parseExpr()
				if err != nil {
					return err
				}
				decl += " = " + e
			} else if nvsKw == "const" {
				return &SyntaxError{Lang: "js", Line: p.peek().line, Detail: "const declaration requires an initializer"}
			}
			p.e.line(decl)
			p.define(name)
		}
		if !p.acceptOp(",") {
			break
		}
	}
	p.semi()
	return nil
}

func (p *jsParser) parseDestructureArr(kw string) error {
	p.next() // [
	var names []string
	for {
		if p.peek().kind != jsName {
			return p.unsupported("array destructuring pattern", "only simple names are supported in destructuring")
		}
		names = append(names, p.next().text)
		if !p.acceptOp(",") {
			break
		}
		if p.peek().kind == jsOp && p.peek().text == "]" {
			break
		}
	}
	if err := p.expectOp("]"); err != nil {
		return err
	}
	if err := p.expectOp("="); err != nil {
		return err
	}
	e, err := p.parseExpr()
	if err != nil {
		return err
	}
	p.e.line(kw + " [" + strings.Join(names, ", ") + "] = " + e)
	for _, n := range names {
		p.define(n)
	}
	return nil
}

func (p *jsParser) parseDestructureObj(kw string) error {
	p.next() // {
	var pairs [][2]string
	for {
		if p.peek().kind != jsName && p.peek().kind != jsStr {
			return p.unsupported("object destructuring pattern", "only simple names are supported in destructuring")
		}
		key := p.next()
		kname := key.text
		if key.kind == jsStr {
			kname = key.val
		}
		vname := kname
		if p.acceptOp(":") {
			if p.peek().kind != jsName {
				return p.unsupported("object destructuring pattern", "only simple names are supported in destructuring")
			}
			vname = p.next().text
		}
		pairs = append(pairs, [2]string{kname, vname})
		if !p.acceptOp(",") {
			break
		}
		if p.peek().kind == jsOp && p.peek().text == "}" {
			break
		}
	}
	if err := p.expectOp("}"); err != nil {
		return err
	}
	if err := p.expectOp("="); err != nil {
		return err
	}
	e, err := p.parseExpr()
	if err != nil {
		return err
	}
	// NvS: let {x, y} = ... / let {k: renamed} = ...
	var parts []string
	for _, pr := range pairs {
		if pr[0] == pr[1] {
			parts = append(parts, pr[0])
		} else {
			parts = append(parts, pr[0]+": "+pr[1])
		}
		p.define(pr[1])
	}
	p.e.line(kw + " {" + strings.Join(parts, ", ") + "} = " + e)
	return nil
}

func (p *jsParser) parseFuncDecl() error {
	p.next() // function
	if p.acceptOp("*") {
		return p.unsupported("generator function", "generators are outside the importable subset")
	}
	name := ""
	if p.peek().kind == jsName {
		name = p.next().text
	}
	params, err := p.parseParams()
	if err != nil {
		return err
	}
	if err := p.expectOp("{"); err != nil {
		return err
	}
	fnkw := "fn "
	if name != "" {
		fnkw += name
		p.define(name)
	}
	p.e.open(fnkw + "(" + strings.Join(params, ", ") + ")")
	p.scopes = append(p.scopes, map[string]bool{})
	for _, pr := range params {
		p.define(strings.Split(pr, "=")[0])
	}
	if err := p.parseBlockBody(); err != nil {
		return err
	}
	p.scopes = p.scopes[:len(p.scopes)-1]
	p.e.close()
	return nil
}

func (p *jsParser) parseParams() ([]string, error) {
	if err := p.expectOp("("); err != nil {
		return nil, err
	}
	var params []string
	if !p.acceptOp(")") {
		for {
			if p.acceptOp("...") {
				return nil, p.unsupported("rest parameters", "rest parameters are outside the importable subset")
			}
			if p.peek().kind != jsName {
				return nil, &SyntaxError{Lang: "js", Line: p.peek().line, Detail: "expected parameter name"}
			}
			pname := p.next().text
			param := pname
			if p.acceptOp("=") {
				dflt, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				param = pname + "=" + dflt
			}
			params = append(params, param)
			if !p.acceptOp(",") {
				break
			}
			if p.peek().kind == jsOp && p.peek().text == ")" {
				break
			}
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
	}
	return params, nil
}

func (p *jsParser) parseIf() error {
	p.next() // if
	if err := p.expectOp("("); err != nil {
		return err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return err
	}
	if err := p.expectOp(")"); err != nil {
		return err
	}
	p.e.open("if (" + cond + ")")
	if err := p.parseStmtOrBlock(); err != nil {
		return err
	}
	for p.acceptName("else") {
		if p.peek().kind == jsName && p.peek().text == "if" {
			p.next()
			if err := p.expectOp("("); err != nil {
				return err
			}
			cond, err := p.parseExpr()
			if err != nil {
				return err
			}
			if err := p.expectOp(")"); err != nil {
				return err
			}
			p.e.closeElse("else if (" + cond + ")")
			if err := p.parseStmtOrBlock(); err != nil {
				return err
			}
		} else {
			p.e.closeElse("else")
			if err := p.parseStmtOrBlock(); err != nil {
				return err
			}
			break
		}
	}
	p.e.close()
	return nil
}

// parseStmtOrBlock parses a single statement or block WITHOUT extra braces
// when the emitter level is already opened.
func (p *jsParser) parseStmtOrBlock() error {
	if p.peek().kind == jsOp && p.peek().text == "{" {
		p.next() // {
		return p.parseBlockBody()
	}
	return p.parseStmt()
}

func (p *jsParser) parseWhile() error {
	p.next() // while
	if err := p.expectOp("("); err != nil {
		return err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return err
	}
	if err := p.expectOp(")"); err != nil {
		return err
	}
	p.e.open("while (" + cond + ")")
	if err := p.parseStmtOrBlock(); err != nil {
		return err
	}
	p.e.close()
	return nil
}

func (p *jsParser) parseFor() error {
	p.next() // for
	if err := p.expectOp("("); err != nil {
		return err
	}
	// for-of / for-in detection: [let|const|var] name of|in expr
	save := p.pos
	if t := p.peek(); t.kind == jsName && (t.text == "let" || t.text == "const" || t.text == "var") {
		p.next()
		if p.peek().kind == jsName {
			vname := p.next().text
			if p.peek().kind == jsName && p.peek().text == "of" {
				p.next()
				iter, err := p.parseExpr()
				if err != nil {
					return err
				}
				if err := p.expectOp(")"); err != nil {
					return err
				}
				p.e.open("for (" + vname + " in " + iter + ")")
				p.define(vname)
				if err := p.parseStmtOrBlock(); err != nil {
					return err
				}
				p.e.close()
				return nil
			}
			if p.peek().kind == jsName && p.peek().text == "in" {
				return p.unsupported("for-in over objects", "JS for-in enumerates keys; NvS for-in iterates values — semantics differ")
			}
		}
		p.pos = save
	} else if t.kind == jsName {
		// for (x of arr)
		p.next()
		if p.peek().kind == jsName && p.peek().text == "of" {
			vname := t.text
			p.next()
			iter, err := p.parseExpr()
			if err != nil {
				return err
			}
			if err := p.expectOp(")"); err != nil {
				return err
			}
			p.e.open("for (" + vname + " in " + iter + ")")
			p.define(vname)
			if err := p.parseStmtOrBlock(); err != nil {
				return err
			}
			p.e.close()
			return nil
		}
		p.pos = save
	}
	// C-style for: init; cond; post
	var initStr string
	if p.peek().kind == jsOp && p.peek().text == ";" {
		initStr = ""
	} else if t := p.peek(); t.kind == jsName && (t.text == "let" || t.text == "var") {
		kw := p.next().text
		_ = kw
		if p.peek().kind != jsName {
			return &SyntaxError{Lang: "js", Line: p.peek().line, Detail: "expected loop variable name"}
		}
		vname := p.next().text
		initStr = "let " + vname
		if p.acceptOp("=") {
			e, err := p.parseExpr()
			if err != nil {
				return err
			}
			initStr += " = " + e
		}
		p.define(vname)
	} else {
		e, err := p.parseExpr()
		if err != nil {
			return err
		}
		initStr = e
	}
	if err := p.expectOp(";"); err != nil {
		return err
	}
	cond := "true"
	if !(p.peek().kind == jsOp && p.peek().text == ";") {
		c, err := p.parseExpr()
		if err != nil {
			return err
		}
		cond = c
	}
	if err := p.expectOp(";"); err != nil {
		return err
	}
	post := ""
	if !(p.peek().kind == jsOp && p.peek().text == ")") {
		pp, err := p.parseForPost()
		if err != nil {
			return err
		}
		post = pp
	}
	if err := p.expectOp(")"); err != nil {
		return err
	}
	p.e.open("for (" + initStr + "; " + cond + "; " + post + ")")
	if err := p.parseStmtOrBlock(); err != nil {
		return err
	}
	p.e.close()
	return nil
}

// parseForPost parses the for-post expression, rewriting i++/i--/++i/--i.
func (p *jsParser) parseForPost() (string, error) {
	t := p.peek()
	if t.kind == jsName {
		name := t.text
		if p.peek2().kind == jsOp && (p.peek2().text == "++" || p.peek2().text == "--") {
			p.next()
			op := p.next().text
			delta := "+ 1"
			if op == "--" {
				delta = "- 1"
			}
			return name + " = " + name + " " + delta, nil
		}
	}
	if t.kind == jsOp && (t.text == "++" || t.text == "--") {
		p.next()
		if p.peek().kind != jsName {
			return "", p.unsupported("for-post expression", "only i++, i--, ++i, --i or assignments are supported as for-post")
		}
		name := p.next().text
		delta := "+ 1"
		if t.text == "--" {
			delta = "- 1"
		}
		return name + " = " + name + " " + delta, nil
	}
	return p.parseExpr()
}

// --- expressions ---

func (p *jsParser) parseExpr() (string, error) { return p.parseAssign() }

func (p *jsParser) parseAssign() (string, error) {
	left, err := p.parseTernary()
	if err != nil {
		return "", err
	}
	t := p.peek()
	if t.kind == jsOp && (t.text == "=" || t.text == "+=" || t.text == "-=" || t.text == "*=" || t.text == "/=" || t.text == "%=" || t.text == "**=") {
		p.next()
		right, err := p.parseAssign() // right-associative
		if err != nil {
			return "", err
		}
		if !isJsLHS(left) {
			return "", p.unsupported("assignment target", "only names, a[i], and a.b are valid assignment targets")
		}
		op := t.text
		if op == "=" {
			return left + " = " + right, nil
		}
		if op == "**=" {
			return left + " = pow(" + left + ", " + right + ")", nil
		}
		return left + " = (" + left + " " + op[:1] + " (" + right + "))", nil
	}
	return left, nil
}

// isJsLHS is a heuristic: emitted LHS strings from names/members/subscripts.
// jsStrLike reports whether an emitted JS-import expression is known to
// produce a string. Used to propagate JS's string+anything concatenation
// coercion through chained + operators.
func jsStrLike(s string) bool {
	s = strings.TrimSpace(s)
	if isJsStrLit(s) {
		return true
	}
	for _, fn := range []string{"str(", "upper(", "lower(", "trim(", "join(", "repeat("} {
		if strings.HasPrefix(s, fn) {
			return true
		}
	}
	if len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' && jsOuterParensMatch(s) {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if jsStrLike(inner) {
			return true
		}
		if idx := jsTopLevelPlus(inner); idx >= 0 {
			return jsStrLike(inner[:idx]) || jsStrLike(inner[idx+1:])
		}
		return false
	}
	if idx := jsTopLevelPlus(s); idx >= 0 {
		return jsStrLike(s[:idx]) || jsStrLike(s[idx+1:])
	}
	return false
}

// jsOuterParensMatch reports whether s's first '(' matches its last ')'.
func jsOuterParensMatch(s string) bool {
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
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i == len(s)-1
			}
		}
	}
	return false
}

// jsTopLevelPlus finds a '+' at paren-depth 0 outside strings, or -1.
func jsTopLevelPlus(s string) int {
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
		case '(':
			depth++
		case ')':
			depth--
		case '+':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// isJsStrLit reports whether s is a double-quoted string literal as emitted
// by this importer (template literals are emitted as "...${...}..." forms,
// which also qualify — interpolation is resolved by NvS itself).
func isJsStrLit(s string) bool {
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

func isJsLHS(s string) bool {
	if s == "" {
		return false
	}
	// NvS-emitted LHS forms: name, name[...], name.attr, calls are not LHS.
	if strings.Contains(s, "(") {
		return false
	}
	c := s[0]
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func (p *jsParser) parseTernary() (string, error) {
	c, err := p.parseNullish()
	if err != nil {
		return "", err
	}
	if p.acceptOp("?") {
		a, err := p.parseExpr()
		if err != nil {
			return "", err
		}
		if err := p.expectOp(":"); err != nil {
			return "", err
		}
		b, err := p.parseTernary()
		if err != nil {
			return "", err
		}
		return "(" + c + ") ? (" + a + ") : (" + b + ")", nil
	}
	return c, nil
}

func (p *jsParser) parseNullish() (string, error) {
	left, err := p.parseOr()
	if err != nil {
		return "", err
	}
	for p.acceptOp("??") {
		right, err := p.parseOr()
		if err != nil {
			return "", err
		}
		left = "(" + left + " ?? " + right + ")"
	}
	return left, nil
}

func (p *jsParser) parseOr() (string, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.acceptOp("||") {
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		left = "(" + left + " or " + right + ")"
	}
	return left, nil
}

func (p *jsParser) parseAnd() (string, error) {
	left, err := p.parseBitOr()
	if err != nil {
		return "", err
	}
	for p.acceptOp("&&") {
		right, err := p.parseBitOr()
		if err != nil {
			return "", err
		}
		left = "(" + left + " and " + right + ")"
	}
	return left, nil
}

func (p *jsParser) parseBitOr() (string, error) {
	left, err := p.parseBitXor()
	if err != nil {
		return "", err
	}
	for p.acceptOp("|") {
		right, err := p.parseBitXor()
		if err != nil {
			return "", err
		}
		left = "(" + left + " | " + right + ")"
	}
	return left, nil
}

func (p *jsParser) parseBitXor() (string, error) {
	left, err := p.parseBitAnd()
	if err != nil {
		return "", err
	}
	for p.acceptOp("^") {
		right, err := p.parseBitAnd()
		if err != nil {
			return "", err
		}
		left = "(" + left + " ^ " + right + ")"
	}
	return left, nil
}

func (p *jsParser) parseBitAnd() (string, error) {
	left, err := p.parseEquality()
	if err != nil {
		return "", err
	}
	for p.acceptOp("&") {
		right, err := p.parseEquality()
		if err != nil {
			return "", err
		}
		left = "(" + left + " & " + right + ")"
	}
	return left, nil
}

func (p *jsParser) parseEquality() (string, error) {
	left, err := p.parseRel()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind == jsOp && (t.text == "==" || t.text == "!=" || t.text == "===" || t.text == "!==") {
			p.next()
			op := t.text
			if op == "===" {
				op = "=="
			} else if op == "!==" {
				op = "!="
			}
			right, err := p.parseRel()
			if err != nil {
				return "", err
			}
			left = "(" + left + " " + op + " " + right + ")"
			continue
		}
		return left, nil
	}
}

func (p *jsParser) parseRel() (string, error) {
	left, err := p.parseShift()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind == jsOp && (t.text == "<" || t.text == ">" || t.text == "<=" || t.text == ">=") {
			p.next()
			right, err := p.parseShift()
			if err != nil {
				return "", err
			}
			left = "(" + left + " " + t.text + " " + right + ")"
			continue
		}
		if t.kind == jsName && (t.text == "instanceof" || t.text == "in") {
			return "", p.unsupported(t.text+" operator", "instanceof/in are outside the importable subset")
		}
		return left, nil
	}
}

func (p *jsParser) parseShift() (string, error) {
	left, err := p.parseAdd()
	if err != nil {
		return "", err
	}
	if t := p.peek(); t.kind == jsOp && (t.text == "<<" || t.text == ">>" || t.text == ">>>") {
		return "", p.unsupported("bit shift", "shift operators are outside the importable subset")
	}
	return left, nil
}

func (p *jsParser) parseAdd() (string, error) {
	left, err := p.parseMul()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind == jsOp && (t.text == "+" || t.text == "-") {
			p.next()
			right, err := p.parseMul()
			if err != nil {
				return "", err
			}
			if t.text == "+" {
				// JS coerces: string + anything is concatenation. NvS
				// rejects mixed + at runtime, so wrap the non-string
				// side in str() when a string is present. Stringness
				// propagates left through a + chain, matching JS
				// left-associativity (1+2+"x" -> "3x", not "12x").
				// (Divergence: arrays/objects stringify differently.)
				lStr, rStr := jsStrLike(left), jsStrLike(right)
				switch {
				case lStr && !rStr:
					left = "(" + left + " + str(" + right + "))"
					continue
				case rStr && !lStr:
					left = "(str(" + left + ") + " + right + ")"
					continue
				}
			}
			left = "(" + left + " " + t.text + " " + right + ")"
			continue
		}
		return left, nil
	}
}

func (p *jsParser) parseMul() (string, error) {
	left, err := p.parseUnary()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind == jsOp && (t.text == "*" || t.text == "/" || t.text == "%") {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return "", err
			}
			left = "(" + left + " " + t.text + " " + right + ")"
			continue
		}
		return left, nil
	}
}

func (p *jsParser) parseUnary() (string, error) {
	t := p.peek()
	if t.kind == jsOp {
		switch t.text {
		case "!", "-", "+", "~":
			p.next()
			e, err := p.parseUnary()
			if err != nil {
				return "", err
			}
			if t.text == "+" {
				return e, nil
			}
			if t.text == "!" {
				return "(!(" + e + "))", nil
			}
			return "(" + t.text + "(" + e + "))", nil
		case "++", "--":
			return "", p.unsupported("prefix "+t.text, "prefix ++/-- are outside the importable subset (use x = x +/- 1)")
		}
	}
	if t.kind == jsName && (t.text == "typeof" || t.text == "void" || t.text == "delete") {
		return "", p.unsupported(t.text+" operator", t.text+" is outside the importable subset")
	}
	return p.parsePower()
}

func (p *jsParser) parsePower() (string, error) {
	base, err := p.parsePostfix()
	if err != nil {
		return "", err
	}
	if p.acceptOp("**") {
		exp, err := p.parseUnary()
		if err != nil {
			return "", err
		}
		return "pow(" + base + ", " + exp + ")", nil
	}
	return base, nil
}

func (p *jsParser) parsePostfix() (string, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return "", err
	}
	for {
		t := p.peek()
		if t.kind != jsOp {
			return node, nil
		}
		switch t.text {
		case "?.":
			return "", p.unsupported("optional chaining", "optional chaining (?.) is outside the importable subset")
		case ".":
			p.next()
			if p.peek().kind != jsName {
				return "", &SyntaxError{Lang: "js", Line: p.peek().line, Detail: "expected property name after '.'"}
			}
			prop := p.next().text
			if p.peek().kind == jsOp && p.peek().text == "(" {
				// method call
				args, err := p.parseCallArgs()
				if err != nil {
					return "", err
				}
				node, err = p.parseKnownMethod(node, prop, args)
				if err != nil {
					return "", err
				}
			} else if prop == "length" {
				node = "len(" + node + ")"
			} else {
				// Plain property access passes through: NvS hashes
				// support o.a as well as o["a"].
				node = node + "." + prop
			}
		case "[":
			p.next()
			idx, err := p.parseExpr()
			if err != nil {
				return "", err
			}
			if err := p.expectOp("]"); err != nil {
				return "", err
			}
			node = node + "[" + idx + "]"
		case "(":
			args, err := p.parseCallArgs()
			if err != nil {
				return "", err
			}
			node = p.applyCall(node, args)
			if node == "" {
				return "", errCallRej
			}
		case "++", "--":
			return "", p.unsupported("postfix "+t.text, "postfix ++/-- are outside the importable subset (use x = x +/- 1)")
		default:
			return node, nil
		}
	}
}

// parseKnownMethod maps the small whitelist of supported method calls;
// everything else is rejected loudly.
func (p *jsParser) parseKnownMethod(node, prop string, args []string) (string, error) {
	if node == "console" && prop == "log" {
		return p.applyCall("console.log", args), nil
	}
	switch prop {
	case "push":
		if len(args) == 1 {
			return "push(" + node + ", " + args[0] + ")", nil
		}
	case "pop":
		if len(args) == 0 {
			return "pop(" + node + ")", nil
		}
	case "join":
		// JS default separator is ",".
		if len(args) == 0 {
			return `join(` + node + `, ",")`, nil
		}
		if len(args) == 1 {
			return "join(" + node + ", " + args[0] + ")", nil
		}
	case "split":
		if len(args) == 1 {
			return "split(" + node + ", " + args[0] + ")", nil
		}
	case "slice":
		if len(args) == 1 {
			return "slice(" + node + ", " + args[0] + ", len(" + node + "))", nil
		}
		if len(args) == 2 {
			return "slice(" + node + ", " + args[0] + ", " + args[1] + ")", nil
		}
	case "indexOf":
		if len(args) == 1 {
			return "index_of(" + node + ", " + args[0] + ")", nil
		}
	case "includes":
		if len(args) == 1 {
			return "(index_of(" + node + ", " + args[0] + ") != -1)", nil
		}
	case "map", "filter":
		if len(args) == 1 {
			return prop + "(" + node + ", " + args[0] + ")", nil
		}
	case "toUpperCase":
		if len(args) == 0 {
			return "upper(" + node + ")", nil
		}
	case "toLowerCase":
		if len(args) == 0 {
			return "lower(" + node + ")", nil
		}
	case "trim":
		if len(args) == 0 {
			return "trim(" + node + ")", nil
		}
	}
	return "", p.unsupported("method call ."+prop+"()", "only .push(x), .pop(), .join(), .split(), .slice(), .indexOf(), .includes(), .map(), .filter(), .toUpperCase(), .toLowerCase(), .trim(), and console.log(...) are supported")
}

// errCallRej is a sentinel: applyCall returns "" only when it already
// produced the error via unsupported(); callers check node == "".
var errCallRej = fmt.Errorf("call rejected")

// applyCall maps known callees (console.log, Math.*) and passes the rest
// through. Returns "" when the call is rejected (error already built by
// the caller via p.unsupported — see parsePostfix).
func (p *jsParser) applyCall(node string, args []string) string {
	argStr := strings.Join(args, ", ")
	switch node {
	case "console.log":
		switch len(args) {
		case 0:
			return `print("")`
		case 1:
			return "print(" + args[0] + ")"
		default:
			strs := make([]string, len(args))
			for i, a := range args {
				strs[i] = "str(" + a + ")"
			}
			return `print(join([` + strings.Join(strs, ", ") + `], " "))`
		}
	}
	if strings.HasPrefix(node, "Math.") {
		m := strings.TrimPrefix(node, "Math.")
		switch m {
		case "max", "min", "abs", "floor", "ceil", "round", "sqrt", "pow", "random":
			return m + "(" + argStr + ")"
		}
	}
	return node + "(" + argStr + ")"
}

func (p *jsParser) parseCallArgs() ([]string, error) {
	p.next() // (
	var args []string
	if !p.acceptOp(")") {
		for {
			if p.acceptOp("...") {
				e, err := p.parseAssign()
				if err != nil {
					return nil, err
				}
				args = append(args, "..."+e)
			} else {
				e, err := p.parseAssign()
				if err != nil {
					return nil, err
				}
				args = append(args, e)
			}
			if !p.acceptOp(",") {
				break
			}
			if p.peek().kind == jsOp && p.peek().text == ")" {
				break
			}
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
	}
	return args, nil
}

func (p *jsParser) parsePrimary() (string, error) {
	t := p.peek()
	switch t.kind {
	case jsNum:
		p.next()
		return t.text, nil
	case jsStr:
		p.next()
		if t.template {
			return p.convertTemplate(t.val, t.line)
		}
		return nvsQuote(t.val), nil
	case jsName:
		switch t.text {
		case "await":
			return "", p.unsupported("await expression", "async/await is outside the importable subset")
		case "true", "false", "null":
			p.next()
			return t.text, nil
		case "undefined":
			p.next()
			return "null", nil // documented divergence
		case "function":
			return p.parseFuncExpr()
		case "this":
			return "", p.unsupported("this", "this-binding is outside the importable subset")
		case "super", "new":
			return "", p.unsupported(t.text, t.text+" is outside the importable subset")
		case "class":
			return "", p.unsupported("class expression", "classes are outside the importable subset")
		}
		// Arrow with single param: name => ...
		if p.peek2().kind == jsOp && p.peek2().text == "=>" {
			pname := p.next().text
			p.next() // =>
			return p.parseArrowBody([]string{pname})
		}
		p.next()
		return t.text, nil
	case jsOp:
		switch t.text {
		case "(":
			if p.isArrowAhead() {
				return p.parseArrow()
			}
			p.next()
			e, err := p.parseExpr()
			if err != nil {
				return "", err
			}
			if err := p.expectOp(")"); err != nil {
				return "", err
			}
			return "(" + e + ")", nil
		case "[":
			return p.parseArray()
		case "{":
			return p.parseObject()
		case "new":
			return "", p.unsupported("new", "constructors are outside the importable subset")
		}
	}
	if t.kind == jsOp && t.text == "/" {
		// A "/" where an operand is expected is a regex literal (the lexer
		// emits division tokens; real division never reaches here).
		return "", p.unsupported("regex literal", "regular expressions are outside the importable subset")
	}
	return "", &SyntaxError{Lang: "js", Line: t.line, Detail: fmt.Sprintf("unexpected %s in expression", t)}
}

// isArrowAhead reports whether "(...)" is followed by "=>".
func (p *jsParser) isArrowAhead() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		if t.kind == jsOp {
			switch t.text {
			case "(":
				depth++
			case ")":
				depth--
				if depth == 0 {
					if i+1 < len(p.toks) {
						n := p.toks[i+1]
						return n.kind == jsOp && n.text == "=>"
					}
					return false
				}
			case "{":
				return false
			}
		}
	}
	return false
}

func (p *jsParser) parseArrow() (string, error) {
	if err := p.expectOp("("); err != nil {
		return "", err
	}
	var params []string
	if !p.acceptOp(")") {
		for {
			if p.acceptOp("...") {
				return "", p.unsupported("rest parameters", "rest parameters are outside the importable subset")
			}
			if p.peek().kind != jsName {
				return "", &SyntaxError{Lang: "js", Line: p.peek().line, Detail: "expected parameter name"}
			}
			pname := p.next().text
			param := pname
			if p.acceptOp("=") {
				dflt, err := p.parseExpr()
				if err != nil {
					return "", err
				}
				param = pname + "=" + dflt
			}
			params = append(params, param)
			if !p.acceptOp(",") {
				break
			}
		}
		if err := p.expectOp(")"); err != nil {
			return "", err
		}
	}
	if err := p.expectOp("=>"); err != nil {
		return "", err
	}
	return p.parseArrowBody(params)
}

func (p *jsParser) parseArrowBody(params []string) (string, error) {
	pname := func(pr string) string { return strings.Split(pr, "=")[0] }
	if p.peek().kind == jsOp && p.peek().text == "{" {
		// Block body: capture statements via a sub-emitter.
		p.next() // {
		sub := &jsParser{toks: p.toks, pos: p.pos, e: &emitter{}, scopes: []map[string]bool{{}}}
		for _, pr := range params {
			sub.define(pname(pr))
		}
		// Share position: parse statements until "}".
		sub.e.ind = 1
		for sub.peek().kind != jsEOF && !(sub.peek().kind == jsOp && sub.peek().text == "}") {
			if err := sub.parseStmt(); err != nil {
				return "", err
			}
		}
		if err := sub.expectOp("}"); err != nil {
			return "", err
		}
		p.pos = sub.pos
		body := sub.e.String()
		return "fn(" + strings.Join(params, ", ") + ") {\n" + body + "}", nil
	}
	e, err := p.parseAssign()
	if err != nil {
		return "", err
	}
	return "fn(" + strings.Join(params, ", ") + ") { return " + e + " }", nil
}

func (p *jsParser) parseFuncExpr() (string, error) {
	p.next() // function
	if p.acceptOp("*") {
		return "", p.unsupported("generator function", "generators are outside the importable subset")
	}
	if p.peek().kind == jsName {
		p.next() // name (ignored for anonymous emission)
	}
	params, err := p.parseParams()
	if err != nil {
		return "", err
	}
	if err := p.expectOp("{"); err != nil {
		return "", err
	}
	sub := &jsParser{toks: p.toks, pos: p.pos, e: &emitter{}, scopes: []map[string]bool{{}}}
	for _, pr := range params {
		sub.define(strings.Split(pr, "=")[0])
	}
	sub.e.ind = 1
	for sub.peek().kind != jsEOF && !(sub.peek().kind == jsOp && sub.peek().text == "}") {
		if err := sub.parseStmt(); err != nil {
			return "", err
		}
	}
	if err := sub.expectOp("}"); err != nil {
		return "", err
	}
	p.pos = sub.pos
	return "fn(" + strings.Join(params, ", ") + ") {\n" + sub.e.String() + "}", nil
}

func (p *jsParser) parseArray() (string, error) {
	p.next() // [
	var items []string
	if !p.acceptOp("]") {
		for {
			if p.acceptOp(",") {
				return "", p.unsupported("array holes", "sparse arrays are outside the importable subset")
			}
			if p.acceptOp("...") {
				e, err := p.parseAssign()
				if err != nil {
					return "", err
				}
				items = append(items, "..."+e)
			} else {
				e, err := p.parseAssign()
				if err != nil {
					return "", err
				}
				items = append(items, e)
			}
			if !p.acceptOp(",") {
				break
			}
			if p.peek().kind == jsOp && p.peek().text == "]" {
				break
			}
		}
		if err := p.expectOp("]"); err != nil {
			return "", err
		}
	}
	return "[" + strings.Join(items, ", ") + "]", nil
}

func (p *jsParser) parseObject() (string, error) {
	p.next() // {
	var pairs []string
	if !p.acceptOp("}") {
		for {
			if p.acceptOp("...") {
				e, err := p.parseAssign()
				if err != nil {
					return "", err
				}
				pairs = append(pairs, "..."+e)
			} else {
				t := p.peek()
				var key string
				switch t.kind {
				case jsName:
					key = t.text
					p.next()
				case jsStr:
					if t.template {
						return "", p.unsupported("computed property names", "computed keys are outside the importable subset")
					}
					key = t.val
					p.next()
				case jsNum:
					key = t.text
					p.next()
				default:
					return "", &SyntaxError{Lang: "js", Line: t.line, Detail: fmt.Sprintf("unexpected %s in object literal", t)}
				}
				if p.peek().kind == jsOp && p.peek().text == "(" {
					return "", p.unsupported("object method", "methods in object literals are outside the importable subset")
				}
				var val string
				if p.acceptOp(":") {
					v, err := p.parseAssign()
					if err != nil {
						return "", err
					}
					val = v
				} else {
					// Shorthand {a} -> {"a": a}
					if t.kind != jsName {
						return "", &SyntaxError{Lang: "js", Line: t.line, Detail: "expected ':' after object key"}
					}
					val = key
				}
				pairs = append(pairs, nvsQuote(key)+": "+val)
			}
			if !p.acceptOp(",") {
				break
			}
			if p.peek().kind == jsOp && p.peek().text == "}" {
				break
			}
		}
		if err := p.expectOp("}"); err != nil {
			return "", err
		}
	}
	return "{" + strings.Join(pairs, ", ") + "}", nil
}

// convertTemplate converts a raw template literal body to an NvS
// interpolated string.
func (p *jsParser) convertTemplate(raw string, line int) (string, error) {
	var sb strings.Builder
	sb.WriteString("\"")
	i := 0
	for i < len(raw) {
		if raw[i] == '$' && i+1 < len(raw) && raw[i+1] == '{' {
			j, err := matchBrace(raw, i+1)
			if err != nil {
				return "", &SyntaxError{Lang: "js", Line: line, Detail: "unmatched '${' in template literal"}
			}
			inner := raw[i+2 : j]
			sub, err := p.parseTemplateExpr(inner, line)
			if err != nil {
				return "", err
			}
			sb.WriteString("${" + sub + "}")
			i = j + 1
			continue
		}
		c := raw[i]
		switch c {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		default:
			sb.WriteByte(c)
		}
		i++
	}
	sb.WriteString("\"")
	return sb.String(), nil
}

func (p *jsParser) parseTemplateExpr(inner string, line int) (string, error) {
	toks, err := lexJS(inner)
	if err != nil {
		return "", err
	}
	sub := &jsParser{toks: toks, e: &emitter{}, scopes: p.scopes}
	return sub.parseExpr()
}
