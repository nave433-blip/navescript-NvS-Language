package parser

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

const (
	_ int = iota
	LOWEST
	ASSIGN_PREC // =
	ANDOR       // and/or — looser than == so a == 1 or b == 2 works
	PIPELINE    // |>
	EQUALS      // ==
	RANGE       // .. ...
	LESSGREATER // > or <
	SUM         // +
	PRODUCT     // *
	PREFIX      // -X or !X
	CALL        // myFunction(X)
	INDEX       // array[index]
	POSTFIX     // x++ / x-- — wave 19: binds tightest
)

var precedences = map[lexer.TokenType]int{
	lexer.ASSIGN:         ASSIGN_PREC,
	lexer.PLUS_ASSIGN:    ASSIGN_PREC,
	lexer.MINUS_ASSIGN:   ASSIGN_PREC,
	lexer.STAR_ASSIGN:    ASSIGN_PREC,
	lexer.SLASH_ASSIGN:   ASSIGN_PREC,
	lexer.QUESTION:       ASSIGN_PREC,
	lexer.NULL_COAL:      ASSIGN_PREC,
	lexer.PIPE:           PIPELINE,
	lexer.IN:             EQUALS,
	lexer.EQ:             EQUALS,
	lexer.NOT_EQ:         EQUALS,
	lexer.ELLIPSIS:       RANGE,
	lexer.LT:             LESSGREATER,
	lexer.GT:             LESSGREATER,
	lexer.LTE:            LESSGREATER,
	lexer.GTE:            LESSGREATER,
	lexer.PLUS:           SUM,
	lexer.MINUS:          SUM,
	lexer.SLASH:          PRODUCT,
	lexer.ASTERISK:       PRODUCT,
	lexer.POWER:          PRODUCT + 1,
	lexer.MOD:            PRODUCT,
	lexer.BIT_AND:        PRODUCT,
	lexer.BIT_OR:         PRODUCT,
	lexer.BIT_XOR:        PRODUCT,
	lexer.SHL:            PRODUCT,
	lexer.SHR:            PRODUCT,
	lexer.LPAREN:         CALL,
	lexer.LBRACKET:       INDEX,
	lexer.DOT:            INDEX,
	lexer.DCOLON:         INDEX, // wave 19: a::b / a::\<T\>(...)
	lexer.PLUS_PLUS:      POSTFIX,
	lexer.MINUS_MINUS:     POSTFIX,
	lexer.OPTIONAL_CHAIN: INDEX,
	lexer.AND:            ANDOR,
	lexer.OR:             ANDOR,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l      *lexer.Lexer
	errors []string

	curToken  lexer.Token
	peekToken lexer.Token

	prefixParseFns map[lexer.TokenType]prefixParseFn
	infixParseFns  map[lexer.TokenType]infixParseFn

	// Wave 19: pendingCloses counts extra `>` closes consumed via a `>>`
	// (SHR) token inside nested type-argument lists. `A<B<C>>` lexes the
	// final `>>` as one SHR token; the inner list consumes it and records
	// one pending close so the outer list knows its `>` is already gone.
	pendingCloses int
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	p.prefixParseFns = make(map[lexer.TokenType]prefixParseFn)
	p.registerPrefix(lexer.IDENT, p.parseIdentifier)
	p.registerPrefix(lexer.INT, p.parseIntegerLiteral)
	p.registerPrefix(lexer.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.TRUE, p.parseBoolean)
	p.registerPrefix(lexer.FALSE, p.parseBoolean)
	p.registerPrefix(lexer.NULL, p.parseNullLiteral)
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(lexer.IF, p.parseIfExpression)
	p.registerPrefix(lexer.FUNCTION, p.parseFunctionLiteral)
	p.registerPrefix(lexer.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(lexer.LBRACE, p.parseHashLiteral)
	p.registerPrefix(lexer.NEW, p.parseNewExpression)
	p.registerPrefix(lexer.THIS, p.parseThisExpression)
	p.registerPrefix(lexer.MATCH, p.parseMatchExpression)
	p.registerPrefix(lexer.ELLIPSIS, p.parseSpreadExpression)

	p.infixParseFns = make(map[lexer.TokenType]infixParseFn)
	p.registerInfix(lexer.PLUS, p.parseInfixExpression)
	p.registerInfix(lexer.MINUS, p.parseInfixExpression)
	p.registerInfix(lexer.SLASH, p.parseInfixExpression)
	p.registerInfix(lexer.ASTERISK, p.parseInfixExpression)
	p.registerInfix(lexer.POWER, p.parseInfixExpression)
	p.registerInfix(lexer.MOD, p.parseInfixExpression)
	p.registerInfix(lexer.BIT_AND, p.parseInfixExpression)
	p.registerInfix(lexer.BIT_OR, p.parseInfixExpression)
	p.registerInfix(lexer.BIT_XOR, p.parseInfixExpression)
	p.registerInfix(lexer.SHL, p.parseInfixExpression)
	p.registerInfix(lexer.SHR, p.parseInfixExpression)
	p.registerInfix(lexer.EQ, p.parseInfixExpression)
	p.registerInfix(lexer.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(lexer.LT, p.parseInfixExpression)
	p.registerInfix(lexer.GT, p.parseInfixExpression)
	p.registerInfix(lexer.LTE, p.parseInfixExpression)
	p.registerInfix(lexer.GTE, p.parseInfixExpression)
	p.registerInfix(lexer.AND, p.parseInfixExpression)
	p.registerInfix(lexer.OR, p.parseInfixExpression)
	p.registerInfix(lexer.LPAREN, p.parseCallExpression)
	p.registerInfix(lexer.LBRACKET, p.parseIndexExpression)
	p.registerInfix(lexer.ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.PLUS_ASSIGN, p.parseCompoundAssign)
	p.registerInfix(lexer.MINUS_ASSIGN, p.parseCompoundAssign)
	p.registerInfix(lexer.STAR_ASSIGN, p.parseCompoundAssign)
	p.registerInfix(lexer.SLASH_ASSIGN, p.parseCompoundAssign)
	p.registerInfix(lexer.QUESTION, p.parseTernaryExpression)
	p.registerInfix(lexer.NULL_COAL, p.parseInfixExpression)
	p.registerInfix(lexer.IN, p.parseInfixExpression)
	p.registerInfix(lexer.DOT, p.parseDotInfix)
	p.registerInfix(lexer.DCOLON, p.parseDColonInfix)
	p.registerInfix(lexer.PLUS_PLUS, p.parsePostfixExpression)
	p.registerInfix(lexer.MINUS_MINUS, p.parsePostfixExpression)
	p.registerInfix(lexer.OPTIONAL_CHAIN, p.parseOptionalChainExpression)
	p.registerInfix(lexer.PIPE, p.parsePipelineExpression)
	p.registerInfix(lexer.ELLIPSIS, p.parseExclusiveRangeExpression)

	// Read two tokens so curToken and peekToken are set
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}

	for p.curToken.Type != lexer.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case lexer.LET:
		return p.parseLetStatement()
	case lexer.RETURN:
		return p.parseReturnStatement()
	case lexer.PRINT:
		return p.parsePrintStatement()
	case lexer.WHILE:
		return p.parseWhileStatement()
	case lexer.FUNCTION:
		return p.parseFunctionStatement()
	case lexer.FOR:
		return p.parseForStatement()
	case lexer.BREAK:
		return p.parseBreakStatement()
	case lexer.CONTINUE:
		return p.parseContinueStatement()
	case lexer.IMPORT:
		return p.parseImportStatement()
	case lexer.CLASS:
		return p.parseClassStatement()
	case lexer.CONST:
		return p.parseConstStatement()
	case lexer.YIELD:
		return p.parseYieldStatement()
	case lexer.AT:
		return p.parseDecoratorStatement()
	case lexer.ENUM:
		return p.parseEnumStatement()
	case lexer.RECORD:
		return p.parseRecordStatement()
	case lexer.INTERFACE:
		return p.parseInterfaceStatement()
	case lexer.PUB:
		return p.parsePubStatement()
	case lexer.MODULE:
		return p.parseModuleStatement()
	case lexer.DEFER:
		return p.parseDeferStatement()
	case lexer.THROW:
		return p.parseThrowStatement()
	case lexer.TRY:
		return p.parseTryStatement()
	default:
		// `name:` at statement level is a loop label (a syntax error before wave 1).
		if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
			return p.parseLabeledStatement()
		}
		return p.parseExpressionStatement()
	}
}

// parsePubStatement: `pub` visibility prefix (wave 19, std dialect).
// `pub` is advisory — the interpreter has one flat namespace — but it is
// recorded on the AST for tooling. Supports pub fn/class/let/const/
// interface/enum/record.
func (p *Parser) parsePubStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // consume `pub`
	var stmt ast.Statement
	switch p.curToken.Type {
	case lexer.FUNCTION:
		stmt = p.parseFunctionStatement()
	case lexer.CLASS:
		stmt = p.parseClassStatement()
	case lexer.LET:
		stmt = p.parseLetStatement()
	case lexer.CONST:
		stmt = p.parseConstStatement()
	case lexer.INTERFACE:
		stmt = p.parseInterfaceStatement()
	case lexer.ENUM:
		stmt = p.parseEnumStatement()
	case lexer.RECORD:
		stmt = p.parseRecordStatement()
	default:
		p.errors = append(p.errors, "pub can only prefix fn/class/let/const/interface/enum/record")
		return nil
	}
	if stmt == nil {
		return nil
	}
	// Wave 20: parseClassStatement and friends return concrete pointer
	// types, so a parse failure arrives as a TYPED nil — and the type
	// switch below would panic dereferencing it (e.g. `pub class Bad
	// implements X {}`). A parse error must never be a panic.
	if rv := reflect.ValueOf(stmt); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return nil
	}
	switch n := stmt.(type) {
	case *ast.LetStatement:
		n.Public = true
	case *ast.TypedLetStatement:
		n.Public = true
	case *ast.ConstStatement:
		n.Public = true
	case *ast.ClassStatement:
		n.Public = true
	case *ast.InterfaceDecl:
		n.Public = true
	case *ast.EnumStatement:
		n.Public = true
	case *ast.RecordStatement:
		n.Public = true
	default:
		p.errors = append(p.errors, fmt.Sprintf("pub is not supported on %T (line %d)", stmt, tok.Line))
		return nil
	}
	return stmt
}

// parseModuleStatement: `module std.collections.vector;` (wave 19, std dialect).
// Recorded on the AST; module resolution/loading is a future wave.
func (p *Parser) parseModuleStatement() ast.Statement {
	tok := p.curToken
	name := p.parseDottedPath()
	if name == "" {
		return nil
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return &ast.ModuleStatement{Token: tok, Name: name}
}

// parseDottedPath parses `a.b.c` — curToken is the first token before the
// path (e.g. `module` or `import`). Returns "" on error.
func (p *Parser) parseDottedPath() string {
	if !p.expectPeek(lexer.IDENT) {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(p.curToken.Literal)
	for p.peekTokenIs(lexer.DOT) {
		p.nextToken() // .
		if !p.expectPeek(lexer.IDENT) {
			return ""
		}
		sb.WriteString(".")
		sb.WriteString(p.curToken.Literal)
	}
	return sb.String()
}
func (p *Parser) parseLabeledStatement() ast.Statement {
	label := p.curToken.Literal
	p.nextToken() // consume label
	p.nextToken() // move to the labeled statement
	stmt := p.parseStatement()
	if stmt == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.ForStatement:
		s.Label = label
	case *ast.ForInStatement:
		s.Label = label
	case *ast.WhileStatement:
		s.Label = label
	default:
		p.errors = append(p.errors, "label \""+label+"\" can only be applied to a loop")
		return nil
	}
	return stmt
}

func (p *Parser) parseLetStatement() ast.Statement {
	tok := p.curToken

	// Destructuring: let [a, b] = ... / let {x, y} = ...
	if p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE) {
		return p.parseDestructureLet(tok, false)
	}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	name := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	var typeAnn *ast.TypeAnnotation
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // :
		p.nextToken() // first token of the type
		// Wave 5: full annotations — `let x: int | string = "s"`,
		// `let x: int? = nil`. A union `|` here is annotation-only.
		typeAnn = p.parseTypeAnnotation()
		if typeAnn == nil {
			return nil
		}
	}

	if !p.expectPeek(lexer.ASSIGN) {
		return nil
	}

	p.nextToken()
	value := p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	if typeAnn != nil {
		return &ast.TypedLetStatement{Token: tok, Name: name, Type: typeAnn, Value: value}
	}
	return &ast.LetStatement{Token: tok, Name: name, Value: value}
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{Token: p.curToken}
	p.nextToken()
	stmt.ReturnValue = p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parsePrintStatement() *ast.PrintStatement {
	stmt := &ast.PrintStatement{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	stmt := &ast.WhileStatement{Token: p.curToken}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	// Wave 19: braceless single-statement bodies (`while (c) x++;`).
	stmt.Body = p.parseBodyBlock()
	if stmt.Body == nil {
		return nil
	}
	stmt.OrElse = p.parseLoopElse()
	return stmt
}

func (p *Parser) parseFunctionStatement() ast.Statement {
	// Supports: fn name(params) { body }
	// We desugar it to: let name = fn(params) { body };
	tok := p.curToken

	if !p.expectPeek(lexer.IDENT) {
		// Anonymous function used as expression - fall through shouldn't happen
		return nil
	}
	name := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Wave 19: `fn identity<T>(x: T) -> T` type parameters.
	var typeParams []*ast.TypeParam
	if p.peekTokenIs(lexer.LT) {
		typeParams = p.parseTypeParamList()
		if typeParams == nil {
			return nil
		}
	}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	params, defaults, paramTypes := p.parseFunctionParameters()

	// Wave 5: optional return annotation — `fn name(params): type { body }`.
	// A `:` here was a parse error before, so this is non-breaking.
	// Wave 19: also `-> type` (std dialect).
	returnType := p.parseReturnAnnotation()

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	body := p.parseBlockStatement()

	fn := &ast.FunctionLiteral{
		Token:      tok,
		Parameters: params,
		Defaults:   defaults,
		ParamTypes: paramTypes,
		ReturnType: returnType,
		TypeParams: typeParams,
		Body:       body,
	}

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return &ast.LetStatement{
		Token: tok,
		Name:  name,
		Value: fn,
	}
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	p.nextToken()

	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(lexer.SEMICOLON) && precedence < p.peekPrecedence() {
		// Wave 19: `f<int>(x)` explicit type-argument call. The probe
		// distinguishes it from a less-than comparison; a member target
		// (`v.map<int>(f)`) is rejected in favor of turbofish `::<>`.
		if p.peekTokenIs(lexer.LT) {
			if _, ok := leftExp.(*ast.MemberExpression); ok && p.probeTypeArgsCall() {
				p.errors = append(p.errors,
					"explicit type arguments on method calls use turbofish syntax: v.map::<T>(...)")
				return nil
			}
			if _, ok := leftExp.(*ast.Identifier); ok && p.probeTypeArgsCall() {
				targs := p.parseTypeArgList()
				if targs == nil {
					return nil
				}
				if !p.expectPeek(lexer.LPAREN) {
					p.errors = append(p.errors, "type arguments must be followed by a call: f<T>(...)")
					return nil
				}
				call := &ast.CallExpression{Token: p.curToken, Function: leftExp, TypeArgs: targs}
				call.Arguments = p.parseCallArguments()
				leftExp = call
				continue
			}
		}
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}
	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as integer", p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
	lit.Value = value
	return lit
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{Token: p.curToken}
	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as float", p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
	lit.Value = value
	return lit
}

func (p *Parser) parseBoolean() ast.Expression {
	return &ast.Boolean{Token: p.curToken, Value: p.curTokenIs(lexer.TRUE)}
}

func (p *Parser) parseNullLiteral() ast.Expression {
	return &ast.NullLiteral{Token: p.curToken}
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expression.Right = p.parseExpression(PREFIX)
	return expression
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	op := p.curToken.Literal
	// Normalize keyword forms
	if op == "and" {
		op = "&&"
	} else if op == "or" {
		op = "||"
	}
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: op,
		Left:     left,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)
	return expression
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	// Wave 3: tuple literals. () is the empty tuple; (e,) is a 1-tuple;
	// (a, b, ...) is a tuple. (x) stays a plain grouped expression.
	// This is non-breaking: `(` expr `,` `)` never parsed before.
	tok := p.curToken
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken() // consume )
		return &ast.TupleLiteral{Token: tok, Elements: []ast.Expression{}}
	}
	p.nextToken()
	exp := p.parseExpression(LOWEST)
	if exp == nil {
		return nil
	}
	if p.peekTokenIs(lexer.COMMA) {
		elements := []ast.Expression{exp}
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken() // consume comma
			if p.peekTokenIs(lexer.RPAREN) {
				break // trailing comma: (1,)
			}
			p.nextToken()
			e := p.parseExpression(LOWEST)
			if e == nil {
				return nil
			}
			elements = append(elements, e)
		}
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
		return &ast.TupleLiteral{Token: tok, Elements: elements}
	}
	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}
	return exp
}

func (p *Parser) parseIfExpression() ast.Expression {
	expression := &ast.IfExpression{Token: p.curToken}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	p.nextToken()
	expression.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	expression.Consequence = p.parseBodyBlock()
	if expression.Consequence == nil {
		return nil
	}

	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken()
		// else if ...
		if p.peekTokenIs(lexer.IF) {
			p.nextToken()
			expression.Alternative = &ast.BlockStatement{
				Statements: []ast.Statement{
					&ast.ExpressionStatement{Expression: p.parseIfExpression()},
				},
			}
		} else {
			expression.Alternative = p.parseBodyBlock()
			if expression.Alternative == nil {
				return nil
			}
		}
	}

	return expression
}

// parseBodyBlock parses a `{ ... }` block or — wave 19 (std dialect) — a
// single statement without braces (`if (c) return x;`). The single statement
// is wrapped in a BlockStatement so the evaluator needs no changes.
// Call with curToken on the token before the body (`)` of the condition).
func (p *Parser) parseBodyBlock() *ast.BlockStatement {
	if p.peekTokenIs(lexer.LBRACE) {
		p.nextToken() // consume {
		return p.parseBlockStatement()
	}
	p.nextToken() // move to the single statement
	tok := p.curToken
	stmt := p.parseStatement()
	if stmt == nil {
		return nil
	}
	return &ast.BlockStatement{Token: tok, Statements: []ast.Statement{stmt}}
}

func (p *Parser) parseFunctionLiteral() ast.Expression {
	lit := &ast.FunctionLiteral{Token: p.curToken}

	// Wave 19: `fn<T>(x: T) -> T` type parameters on anonymous literals.
	if p.peekTokenIs(lexer.LT) {
		lit.TypeParams = p.parseTypeParamList()
		if lit.TypeParams == nil {
			return nil
		}
	}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	lit.Parameters, lit.Defaults, lit.ParamTypes = p.parseFunctionParameters()

	// Wave 5: optional return annotation — `fn (params): type { body }`.
	// Wave 19: also `-> type` (std dialect).
	lit.ReturnType = p.parseReturnAnnotation()

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()
	return lit
}

func (p *Parser) parseFunctionParameters() ([]*ast.Identifier, []ast.Expression, []*ast.TypeAnnotation) {
	identifiers := []*ast.Identifier{}
	defaults := []ast.Expression{}
	types := []*ast.TypeAnnotation{}

	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return identifiers, defaults, types
	}

	p.nextToken()
	ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	identifiers = append(identifiers, ident)
	types = append(types, p.parseParamAnnotation())
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // =
		p.nextToken()
		defaults = append(defaults, p.parseExpression(LOWEST))
	} else {
		defaults = append(defaults, nil)
	}

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()
		ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		identifiers = append(identifiers, ident)
		types = append(types, p.parseParamAnnotation())
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken()
			p.nextToken()
			defaults = append(defaults, p.parseExpression(LOWEST))
		} else {
			defaults = append(defaults, nil)
		}
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil, nil, nil
	}

	return identifiers, defaults, types
}

// parseParamAnnotation parses an optional `: type` annotation following a
// parameter name. Called with curToken on the parameter identifier; on
// return curToken is on the last token of the annotation (or still on the
// identifier when there is none). Returns nil when no annotation is present.
func (p *Parser) parseParamAnnotation() *ast.TypeAnnotation {
	if !p.peekTokenIs(lexer.COLON) {
		return nil
	}
	p.nextToken() // :
	p.nextToken() // first token of the type
	return p.parseTypeAnnotation()
}

// parseTypeAnnotation parses a type annotation: `int`, `int | string`,
// `Shape`, `int?` (`?` desugars to `| null`). Called with curToken on the
// first token of the type; the `|` spelling is only special here — everywhere
// else it keeps its usual (bitwise-or) meaning.
func (p *Parser) parseTypeAnnotation() *ast.TypeAnnotation {
	// Wave 19: `fn(A, B) -> R` function-type annotations.
	if p.curTokenIs(lexer.FUNCTION) {
		return p.parseFnTypeAnnotation()
	}
	// `null` is a keyword token, but it's a legitimate type name.
	// Wave 19: `[T]` array sugar is also a valid start.
	if !p.curTokenIs(lexer.IDENT) && !p.curTokenIs(lexer.NULL) && !p.curTokenIs(lexer.LBRACKET) {
		p.errors = append(p.errors, "expected type name in annotation")
		return nil
	}
	tok := p.curToken
	members := []*ast.TypeAnnotation{p.parseSingleTypeAnnotation()}
	if members[0] == nil {
		return nil
	}
	// `T?` sugar for `T | null`.
	if p.peekTokenIs(lexer.QUESTION) {
		p.nextToken()
		members = append(members, &ast.TypeAnnotation{Token: p.curToken, Name: "null"})
	}
	for p.peekTokenIs(lexer.BIT_OR) {
		p.nextToken() // |
		p.nextToken() // first token of the member type
		m := p.parseSingleTypeAnnotation()
		if m == nil {
			return nil
		}
		members = append(members, m)
		if p.peekTokenIs(lexer.QUESTION) {
			p.nextToken()
			members = append(members, &ast.TypeAnnotation{Token: p.curToken, Name: "null"})
		}
	}
	if len(members) == 1 {
		return members[0]
	}
	return &ast.TypeAnnotation{Token: tok, Union: members}
}

// parseSingleTypeAnnotation parses one non-union type: `int`, `Vec<T>`,
// `Map<string, int>`, `string[]`, `[string]`. curToken is the first token
// of the type; on success curToken is the last token of the type.
func (p *Parser) parseSingleTypeAnnotation() *ast.TypeAnnotation {
	// Wave 19: `fn(A, B) -> R` function-type annotations.
	if p.curTokenIs(lexer.FUNCTION) {
		return p.parseFnTypeAnnotation()
	}
	// Wave 19: `[T]` array sugar (equivalent to `T[]`; the sketch uses both).
	if p.curTokenIs(lexer.LBRACKET) {
		p.nextToken() // [
		elem := p.parseTypeAnnotation()
		if elem == nil {
			return nil
		}
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		elem.IsArray = true
		return elem
	}
	if !p.curTokenIs(lexer.IDENT) && !p.curTokenIs(lexer.NULL) {
		p.errors = append(p.errors, "expected type name in annotation")
		return nil
	}
	tok := p.curToken
	ann := &ast.TypeAnnotation{Token: tok, Name: tok.Literal}
	// Wave 19: parameterized types — `Vec<int>`, `Map<string, Vec<int>>`.
	if p.peekTokenIs(lexer.LT) {
		ann.TypeArgs = p.parseTypeArgList()
		if ann.TypeArgs == nil {
			return nil
		}
	}
	// Wave 19: `T[]` array sugar — `string[]`. A `[` here was a parse error
	// before, so requiring `[]` is non-breaking.
	if p.peekTokenIs(lexer.LBRACKET) {
		p.nextToken() // [
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		ann.IsArray = true
	}
	return ann
}

// parseFnTypeAnnotation parses `fn(A, B) -> R` / `fn(A, B): R` — wave 19.
// curToken is `fn`; on success curToken is the last token of the return type
// (or `)` when unannotated).
func (p *Parser) parseFnTypeAnnotation() *ast.TypeAnnotation {
	tok := p.curToken
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	sig := &ast.FnTypeSig{}
	if !p.peekTokenIs(lexer.RPAREN) {
		for {
			p.nextToken() // first token of the parameter type
			pt := p.parseTypeAnnotation()
			if pt == nil {
				return nil
			}
			sig.Params = append(sig.Params, pt)
			if p.peekTokenIs(lexer.COMMA) {
				p.nextToken()
				continue
			}
			break
		}
	}
	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}
	sig.Return = p.parseReturnAnnotation()
	return &ast.TypeAnnotation{Token: tok, Name: "function", FnSig: sig}
}

func (p *Parser) parseCallExpression(function ast.Expression) ast.Expression {
	exp := &ast.CallExpression{Token: p.curToken, Function: function}
	exp.Arguments = p.parseCallArguments()
	return exp
}

func (p *Parser) parseCompoundAssign(left ast.Expression) ast.Expression {
	ident, ok := left.(*ast.Identifier)
	if !ok {
		p.errors = append(p.errors, "compound assignment requires identifier")
		return nil
	}
	op := p.curToken.Literal
	tok := p.curToken
	p.nextToken()
	right := p.parseExpression(LOWEST)
	baseOp := string(op[0])
	return &ast.AssignExpression{
		Token: tok,
		Name:  ident,
		Value: &ast.InfixExpression{
			Token:    tok,
			Left:     &ast.Identifier{Token: ident.Token, Value: ident.Value},
			Operator: baseOp,
			Right:    right,
		},
	}
}

func (p *Parser) parseTernaryExpression(left ast.Expression) ast.Expression {
	expr := &ast.TernaryExpression{Token: p.curToken, Condition: left}
	p.nextToken()
	expr.Consequence = p.parseExpression(LOWEST)
	if !p.expectPeek(lexer.COLON) {
		return nil
	}
	p.nextToken()
	expr.Alternative = p.parseExpression(LOWEST)
	return expr
}

func (p *Parser) parseAssignExpression(left ast.Expression) ast.Expression {
	if ident, ok := left.(*ast.Identifier); ok {
		expr := &ast.AssignExpression{Token: p.curToken, Name: ident}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	}
	if idx, ok := left.(*ast.IndexExpression); ok {
		expr := &ast.IndexAssignExpression{Token: p.curToken, Left: idx}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	}
	if mem, ok := left.(*ast.MemberExpression); ok {
		expr := &ast.MemberAssignExpression{
			Token:    p.curToken,
			Object:   mem.Object,
			Property: mem.Property,
		}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	}
	p.errors = append(p.errors, "left side of assignment must be identifier, index, or member")
	return nil
}

func (p *Parser) parseArrayLiteral() ast.Expression {
	array := &ast.ArrayLiteral{Token: p.curToken}
	array.Elements = p.parseExpressionList(lexer.RBRACKET)
	return array
}

func (p *Parser) parseExpressionList(end lexer.TokenType) []ast.Expression {
	list := []ast.Expression{}
	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}
	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}
	if !p.expectPeek(end) {
		return nil
	}
	return list
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}
	p.nextToken()
	// slice forms: [:e] [s:e] [s:] [:]
	if p.curTokenIs(lexer.COLON) {
		exp.Index = &ast.IntegerLiteral{Token: p.curToken, Value: 0}
		p.nextToken()
		if p.curTokenIs(lexer.RBRACKET) {
			exp.End = &ast.IntegerLiteral{Token: p.curToken, Value: -1}
			return exp
		}
		exp.End = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		return exp
	}
	exp.Index = p.parseExpression(LOWEST)
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume :
		p.nextToken() // end expr or ]
		if p.curTokenIs(lexer.RBRACKET) {
			exp.End = &ast.IntegerLiteral{Token: p.curToken, Value: -1}
			return exp
		}
		exp.End = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		return exp
	}
	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}
	return exp
}

func (p *Parser) parseForStatement() ast.Statement {
	tok := p.curToken

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	// Lookahead: for (ident in ...)  vs  for (init; cond; post)
	if p.peekTokenIs(lexer.IDENT) {
		// Could be for-in or C-style starting with ident
		// Peek further: after ident, is it "in"?
		// We only have one-token peek, so consume ident and check
		p.nextToken() // ident or let or expr start
		if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.IN) {
			name := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			p.nextToken() // consume 'in'
			p.nextToken() // start of iterable
			iterable := p.parseExpression(LOWEST)
			if !p.expectPeek(lexer.RPAREN) {
				return nil
			}
			// Wave 19: braceless single-statement bodies.
			body := p.parseBodyBlock()
			if body == nil {
				return nil
			}
			stmt := &ast.ForInStatement{
				Token:    tok,
				Name:     name,
				Iterable: iterable,
				Body:     body,
			}
			stmt.OrElse = p.parseLoopElse()
			return stmt
		}
		// Not for-in — fall into C-style. curToken is already first token of init.
		return p.parseCStyleFor(tok, true)
	}

	p.nextToken()
	return p.parseCStyleFor(tok, false)
}

// parseCStyleFor finishes a C-style for. alreadyConsumed indicates curToken is start of init.
func (p *Parser) parseCStyleFor(tok lexer.Token, alreadyConsumed bool) *ast.ForStatement {
	stmt := &ast.ForStatement{Token: tok}

	if !alreadyConsumed {
		// curToken should be start of init or ;
	}

	// Init
	if !p.curTokenIs(lexer.SEMICOLON) {
		if p.curTokenIs(lexer.LET) {
			stmt.Init = p.parseLetStatement()
		} else {
			stmt.Init = p.parseExpressionStatement()
		}
	}
	if !p.curTokenIs(lexer.SEMICOLON) {
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
	}

	// Condition
	p.nextToken()
	if !p.curTokenIs(lexer.SEMICOLON) {
		stmt.Condition = p.parseExpression(LOWEST)
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	// Post
	p.nextToken()
	if !p.curTokenIs(lexer.RPAREN) {
		stmt.Post = p.parseExpression(LOWEST)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}
	// Wave 19: braceless single-statement bodies.
	stmt.Body = p.parseBodyBlock()
	if stmt.Body == nil {
		return nil
	}
	stmt.OrElse = p.parseLoopElse()
	return stmt
}

func (p *Parser) parseYieldStatement() *ast.YieldStatement {
	stmt := &ast.YieldStatement{Token: p.curToken}
	if !p.peekTokenIs(lexer.SEMICOLON) && !p.peekTokenIs(lexer.RBRACE) && p.peekToken.Type != lexer.EOF {
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseDecoratorStatement() ast.Statement {
	stmt := &ast.DecoratorStatement{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Decorator = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	p.nextToken()
	// next should be fn statement
	if p.curTokenIs(lexer.FUNCTION) {
		stmt.Function = p.parseFunctionStatement()
		return stmt
	}
	p.errors = append(p.errors, "@decorator must precede a function")
	return nil
}

func (p *Parser) parseTryStatement() *ast.TryStatement {
	stmt := &ast.TryStatement{Token: p.curToken}
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	if p.peekTokenIs(lexer.CATCH) {
		p.nextToken() // catch
		if p.peekTokenIs(lexer.LPAREN) {
			p.nextToken()
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			stmt.CatchId = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if !p.expectPeek(lexer.RPAREN) {
				return nil
			}
		}
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		stmt.Catch = p.parseBlockStatement()
	}
	if p.peekTokenIs(lexer.FINALLY) {
		p.nextToken()
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		stmt.Finally = p.parseBlockStatement()
	}
	return stmt
}

func (p *Parser) parseThrowStatement() *ast.ThrowStatement {
	stmt := &ast.ThrowStatement{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseConstStatement() ast.Statement {
	tok := p.curToken

	// Destructuring: const [a, b] = ... / const {x, y} = ...
	if p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE) {
		return p.parseDestructureLet(tok, true)
	}

	stmt := &ast.ConstStatement{Token: tok}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	if !p.expectPeek(lexer.ASSIGN) {
		return nil
	}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseMatchExpression() ast.Expression {
	expr := &ast.MatchExpression{Token: p.curToken}
	// Wave 4: the scrutinee parens are optional — `match x { ... }` works
	// exactly like `match (x) { ... }`. (`match` required `(` before, so no
	// previously-valid program changes meaning.)
	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken() // (
		p.nextToken() // first token of scrutinee
		expr.Value = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
	} else {
		p.nextToken() // first token of scrutinee
		expr.Value = p.parseExpression(LOWEST)
		if expr.Value == nil {
			return nil
		}
	}
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	expr.Arms = []*ast.MatchArm{}
	p.nextToken()
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.CASE) {
			p.nextToken()
			arm := &ast.MatchArm{}
			arm.Pattern = p.parseMatchPattern()
			if arm.Pattern == nil {
				return nil
			}
			// Wave 4: optional guard — `case <pat> if <expr>:`
			if p.peekTokenIs(lexer.IF) {
				p.nextToken() // if
				p.nextToken() // first token of the guard
				arm.Guard = p.parseExpression(LOWEST)
				if arm.Guard == nil {
					return nil
				}
			}
			// optional colon
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
			}
			if p.peekTokenIs(lexer.LBRACE) {
				p.nextToken()
				arm.Body = p.parseBlockStatement()
			} else {
				// single-statement arm: case 1: "one" / case 1: print x
				p.nextToken() // move to the first token of the statement
				stmt := p.parseStatement()
				arm.Body = &ast.BlockStatement{
					Statements: []ast.Statement{stmt},
				}
			}
			expr.Arms = append(expr.Arms, arm)
			p.nextToken()
		} else if p.curTokenIs(lexer.DEFAULT) {
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
			}
			if p.peekTokenIs(lexer.LBRACE) {
				p.nextToken()
				expr.Default = p.parseBlockStatement()
			} else {
				// single-statement default: default: "other"
				p.nextToken() // move to the first token of the statement
				stmt := p.parseStatement()
				expr.Default = &ast.BlockStatement{
					Statements: []ast.Statement{stmt},
				}
			}
			p.nextToken()
		} else {
			p.nextToken()
		}
	}
	return expr
}

// parseMatchPattern parses a single `case` pattern (curToken is its first
// token). Most patterns are ordinary expressions — the wave-4 evaluator
// interprets their AST shape as a pattern (identifiers bind, literals test,
// array/tuple/hash literals destructure, `Name(...)` against a declared
// record type is a record pattern, anything else is evaluated and compared).
// The one exception is `{x, y}` / `{k: v}`: not valid hash literals, so they
// are parsed with the wave-1 hash-pattern parser. Anything that is not
// wave-1-shaped (e.g. `{x: 1}`, `{"a": b}`, `{...m}`, `{}`) falls back to a
// plain hash-literal expression with identical meaning to before.
func (p *Parser) parseMatchPattern() ast.Expression {
	if p.curTokenIs(lexer.LBRACE) {
		if pat := p.tryParseHashPattern(); pat != nil {
			return pat
		}
	}
	return p.parseExpression(LOWEST)
}

// tryParseHashPattern speculatively parses a wave-1 `{x, y}` / `{k: v}`
// hash pattern. On any failure the parser state (lexer, tokens, errors) is
// rewound and nil is returned so the caller can try a hash literal instead.
func (p *Parser) tryParseHashPattern() ast.Expression {
	savedLexer := *p.l
	savedCur, savedPeek := p.curToken, p.peekToken
	savedErrors := len(p.errors)
	pat := p.parseHashPattern()
	if pat == nil {
		*p.l = savedLexer
		p.curToken, p.peekToken = savedCur, savedPeek
		p.errors = p.errors[:savedErrors]
		return nil
	}
	// `{}` keeps its old meaning (empty hash literal compared by value);
	// only a non-empty `{...}` becomes a destructuring pattern.
	if hp, ok := pat.(*ast.HashPattern); ok && len(hp.Entries) == 0 {
		*p.l = savedLexer
		p.curToken, p.peekToken = savedCur, savedPeek
		p.errors = p.errors[:savedErrors]
		return nil
	}
	return pat
}

// ---- Wave 5: interfaces ----
// parseInterfaceStatement parses:
//
//	interface Shape {
//	  area(): float
//	  greet(name: string): string
//	}
//
// Method signatures carry parameter names with optional `: type`
// annotations and an optional `: type` return annotation; there are no
// bodies. Members are separated by newlines and/or semicolons.
func (p *Parser) parseInterfaceStatement() *ast.InterfaceDecl {
	stmt := &ast.InterfaceDecl{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	p.nextToken()
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken()
			continue
		}
		// Wave 19: optional `fn` prefix (consistent with classes).
		if p.curTokenIs(lexer.FUNCTION) {
			p.nextToken()
		}
		if !p.curTokenIs(lexer.IDENT) {
			p.errors = append(p.errors, "expected method name in interface body")
			return nil
		}
		method := &ast.InterfaceMethod{Token: p.curToken}
		method.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		// Wave 19 (checker rule 7 / Go's honest boundary): interface methods
		// cannot declare type parameters. Parse-time error, not a silent skip.
		// We consume the <...> and continue so one bad method doesn't
		// cascade into noise.
		if p.peekTokenIs(lexer.LT) {
			p.errors = append(p.errors, "interface methods cannot declare type parameters")
			p.parseTypeParamList()
		}
		if !p.expectPeek(lexer.LPAREN) {
			return nil
		}
		// Parameter list: name[: type], comma-separated. No defaults —
		// a signature declares the contract, not the implementation.
		if !p.peekTokenIs(lexer.RPAREN) {
			for {
				p.nextToken()
				if !p.curTokenIs(lexer.IDENT) {
					p.errors = append(p.errors, "expected parameter name in interface method")
					return nil
				}
				method.ParamNames = append(method.ParamNames,
					&ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
				method.ParamTypes = append(method.ParamTypes, p.parseParamAnnotation())
				if p.peekTokenIs(lexer.COMMA) {
					p.nextToken()
					continue
				}
				break
			}
		}
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
		// Wave 19: `: T` or `-> T` return annotations.
		method.ReturnType = p.parseReturnAnnotation()
		stmt.Methods = append(stmt.Methods, method)
		if p.peekTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
		p.nextToken()
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseClassStatement() *ast.ClassStatement {
	stmt := &ast.ClassStatement{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Wave 19: `class Box<T, U: Bound>` type parameters.
	if p.peekTokenIs(lexer.LT) {
		stmt.TypeParams = p.parseTypeParamList()
		if stmt.TypeParams == nil {
			return nil
		}
	}

	if p.peekTokenIs(lexer.EXTENDS) {
		p.nextToken()
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		stmt.Parent = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	stmt.Methods = []*ast.ClassMethod{}
	stmt.Fields = []*ast.ClassField{}
	p.nextToken()
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		// Wave 19: `pub` visibility on methods and fields (std dialect).
		// Wave 20: `static` method modifier. `static` is a CONTEXTUAL
		// keyword — it only acts as a modifier directly before fn/pub/a
		// method name in a class body, so `static` stays usable as an
		// identifier everywhere else (and a method can even be NAMED
		// `static`: `static() {}` has `(` after it, not fn/pub/IDENT).
		isPub := false
		isStatic := false
		for p.curTokenIs(lexer.PUB) ||
			(p.curTokenIs(lexer.IDENT) && p.curToken.Literal == "static" &&
				(p.peekTokenIs(lexer.FUNCTION) || p.peekTokenIs(lexer.PUB) || p.peekTokenIs(lexer.IDENT))) {
			if p.curTokenIs(lexer.PUB) {
				isPub = true
			} else {
				isStatic = true
			}
			p.nextToken()
		}
		// Wave 19: field declarations — `let x: T;`, `let x: T = v;`
		// (also `const`). A field was a parse error here before.
		if p.curTokenIs(lexer.LET) || p.curTokenIs(lexer.CONST) {
			if isStatic {
				p.errors = append(p.errors, "static fields are not supported (wave 20 supports static methods only)")
				return nil
			}
			field := p.parseClassField(isPub)
			if field == nil {
				return nil
			}
			stmt.Fields = append(stmt.Fields, field)
			p.nextToken()
			continue
		}
		// method: name(params) { body }  or  fn name(params) { body }
		if p.curTokenIs(lexer.FUNCTION) {
			p.nextToken()
		}
		if !p.curTokenIs(lexer.IDENT) {
			p.nextToken()
			continue
		}
		method := &ast.ClassMethod{Token: p.curToken, Public: isPub, Static: isStatic}
		method.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		// Wave 19: method-level type parameters — `fn map<U>(...)`.
		if p.peekTokenIs(lexer.LT) {
			method.TypeParams = p.parseTypeParamList()
			if method.TypeParams == nil {
				return nil
			}
		}
		if !p.expectPeek(lexer.LPAREN) {
			return nil
		}
		method.Parameters, _, method.ParamTypes = p.parseFunctionParameters()
		// Wave 5: optional return annotation on methods — `area(): float { ... }`.
		// Wave 19: also `-> float` (std dialect).
		method.ReturnType = p.parseReturnAnnotation()
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		method.Body = p.parseBlockStatement()
		stmt.Methods = append(stmt.Methods, method)
		p.nextToken()
	}
	return stmt
}

// parseClassField parses `let name[: type] [= value];` inside a class body —
// wave 19 (std dialect). curToken is `let`/`const`; on success curToken is
// the last token of the field (caller advances).
func (p *Parser) parseClassField(isPub bool) *ast.ClassField {
	tok := p.curToken
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	field := &ast.ClassField{
		Token:  tok,
		Name:   &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
		Public: isPub,
	}
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // :
		p.nextToken() // first token of the type
		field.Type = p.parseTypeAnnotation()
		if field.Type == nil {
			return nil
		}
	}
	if p.peekTokenIs(lexer.ASSIGN) {
		p.nextToken() // =
		p.nextToken() // first token of the default
		field.Default = p.parseExpression(LOWEST)
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return field
}

func (p *Parser) parseNewExpression() ast.Expression {
	expr := &ast.NewExpression{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	expr.ClassName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	// Wave 19: `new Box<int>(42)`. The probe keeps `(new Box) < 3`
	// parsing as a comparison: `<` is only type args when `<types>(` follows.
	if p.peekTokenIs(lexer.LT) && p.probeTypeArgsCall() {
		expr.TypeArgs = p.parseTypeArgList()
		if expr.TypeArgs == nil {
			return nil
		}
	}
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	expr.Arguments = p.parseCallArguments()
	return expr
}

// probeTypeArgsCall reports whether the upcoming tokens match
// `<` type (, type)* `>` `(` — i.e. an explicit-type-argument call like
// `f<int>(x)` or `new Box<int>(x)`. Call with curToken on the identifier
// before `<` and peek == LT. Never advances the real parser/lexer state.
//
// This is the standard `f<T>(x)` vs `(f < T) > (x)` disambiguation: when the
// token pattern matches, `<...>` is type arguments; otherwise `<` stays the
// less-than operator (gradual, honest — a weird-but-valid comparison still
// parses as a comparison).
func (p *Parser) probeTypeArgsCall() bool {
	if !p.peekTokenIs(lexer.LT) {
		return false
	}
	// Snapshot the lexer and the two-token window; restore on return.
	savedLexer := *p.l
	savedCur, savedPeek := p.curToken, p.peekToken
	defer func() {
		*p.l = savedLexer
		p.curToken, p.peekToken = savedCur, savedPeek
	}()
	depth := 0
	// Bound the scan: a type-argument list is short; bail out rather than
	// scanning to EOF on adversarial input.
	for i := 0; i < 64; i++ {
		p.nextToken()
		switch p.curToken.Type {
		case lexer.LT:
			depth++
		case lexer.GT:
			depth--
			if depth == 0 {
				return p.peekTokenIs(lexer.LPAREN)
			}
		case lexer.SHR:
			// `>>` closes two levels (see parseTypeArgList).
			depth -= 2
			if depth == 0 {
				return p.peekTokenIs(lexer.LPAREN)
			}
			if depth < 0 {
				return false
			}
		case lexer.IDENT, lexer.NULL, lexer.COMMA, lexer.BIT_OR, lexer.QUESTION,
			lexer.LBRACKET, lexer.RBRACKET, lexer.FUNCTION, lexer.LPAREN, lexer.RPAREN,
			lexer.COLON, lexer.ARROW:
			// Plausible inside a type argument list: names, `|`, `?`,
			// `[]`, `fn(...) -> ...`, bounds with `:`.
		default:
			return false
		}
	}
	return false
}

func (p *Parser) parseThisExpression() ast.Expression {
	return &ast.ThisExpression{Token: p.curToken}
}

func (p *Parser) parseMemberExpression(left ast.Expression) ast.Expression {
	expr := &ast.MemberExpression{Token: p.curToken, Object: left}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	expr.Property = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return expr
}

func (p *Parser) parseBreakStatement() *ast.BreakStatement {
	stmt := &ast.BreakStatement{Token: p.curToken}
	// `break label` — the label must be on the same line, so a `break`
	// followed by a newline and a new statement is never misread.
	if p.peekTokenIs(lexer.IDENT) && p.peekToken.Line == p.curToken.Line {
		p.nextToken()
		stmt.Label = p.curToken.Literal
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseContinueStatement() *ast.ContinueStatement {
	stmt := &ast.ContinueStatement{Token: p.curToken}
	if p.peekTokenIs(lexer.IDENT) && p.peekToken.Line == p.curToken.Line {
		p.nextToken()
		stmt.Label = p.curToken.Literal
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseImportStatement() *ast.ImportStatement {
	stmt := &ast.ImportStatement{Token: p.curToken}
	if p.peekTokenIs(lexer.STRING) {
		p.nextToken()
		stmt.Path = &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	} else {
		// Wave 19: bare module path — `import std.core;` (std dialect).
		// Resolution/loading of module paths is a future wave; the
		// evaluator reports a clean error (see PLACEHOLDERS.md).
		stmt.ModulePath = p.parseDottedPath()
		if stmt.ModulePath == "" {
			return nil
		}
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseHashLiteral() ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken}
	hash.Pairs = make(map[ast.Expression]ast.Expression)

	for !p.peekTokenIs(lexer.RBRACE) {
		// Spread entry: {...m}
		if p.peekTokenIs(lexer.ELLIPSIS) {
			p.nextToken() // ...
			spread := &ast.SpreadExpression{Token: p.curToken}
			p.nextToken()
			spread.Value = p.parseExpression(LOWEST)
			hash.Spreads = append(hash.Spreads, spread)
			if !p.peekTokenIs(lexer.RBRACE) && !p.expectPeek(lexer.COMMA) {
				return nil
			}
			continue
		}
		p.nextToken()
		key := p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.COLON) {
			return nil
		}
		p.nextToken()
		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value
		if !p.peekTokenIs(lexer.RBRACE) && !p.expectPeek(lexer.COMMA) {
			return nil
		}
	}
	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}
	return hash
}

func (p *Parser) parseCallArguments() []ast.Expression {
	args := []ast.Expression{}

	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return args
	}

	// Wave 2: named arguments `name: value` (Python/Kotlin/Swift style).
	// `f(x=1)` stays an assignment expression (backward compat); `name:`
	// is free because COLON is not an infix operator, and a ternary
	// `f(a ? b : c)` consumes its own `:` inside parseTernaryExpression,
	// so a bare trailing Identifier + COLON is unambiguous.
	seenNamed := false
	p.nextToken()
	for {
		arg := p.parseExpression(LOWEST)
		if arg == nil {
			return nil
		}
		if ident, ok := arg.(*ast.Identifier); ok && p.peekTokenIs(lexer.COLON) {
			nameTok := p.curToken
			p.nextToken() // move to COLON
			p.nextToken() // move to first token of the value
			val := p.parseExpression(LOWEST)
			if val == nil {
				return nil
			}
			arg = &ast.NamedArgument{Token: nameTok, Name: ident, Value: val}
			seenNamed = true
		} else if seenNamed {
			// Python rule: positionals first, then named.
			p.errors = append(p.errors, "positional argument follows named argument")
			return nil
		}
		args = append(args, arg)
		if !p.peekTokenIs(lexer.COMMA) {
			break
		}
		p.nextToken() // comma
		p.nextToken() // next argument
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	return args
}

// ---- Wave 1: destructuring ----

// parseDestructureLet: let|const <pattern> = <expr>
// curToken is the let/const token; peek is [ or {.
func (p *Parser) parseDestructureLet(tok lexer.Token, isConst bool) ast.Statement {
	p.nextToken() // move to [ or {
	var pattern ast.Expression
	if p.curTokenIs(lexer.LBRACKET) {
		pattern = p.parseArrayPattern()
	} else {
		pattern = p.parseHashPattern()
	}
	if pattern == nil {
		return nil
	}
	if !p.expectPeek(lexer.ASSIGN) {
		return nil
	}
	p.nextToken()
	value := p.parseExpression(LOWEST)
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return &ast.DestructureLetStatement{Token: tok, IsConst: isConst, Pattern: pattern, Value: value}
}

// parseArrayPattern: [a, b, ...rest] — curToken is `[`.
func (p *Parser) parseArrayPattern() ast.Expression {
	pat := &ast.ArrayPattern{Token: p.curToken}
	if p.peekTokenIs(lexer.RBRACKET) {
		p.nextToken() // empty []
		return pat
	}
	p.nextToken() // first element
	for !p.curTokenIs(lexer.RBRACKET) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.ELLIPSIS) {
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			pat.Rest = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			if !p.peekTokenIs(lexer.RBRACKET) {
				p.errors = append(p.errors, "rest element ..."+pat.Rest.Value+" must be last in array pattern")
				return nil
			}
			p.nextToken() // consume ]
			return pat
		}
		if !p.curTokenIs(lexer.IDENT) {
			p.errors = append(p.errors, "array destructure pattern expects an identifier")
			return nil
		}
		pat.Elements = append(pat.Elements, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			continue
		}
		break
	}
	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}
	return pat
}

// parseHashPattern: {x, y} or {k: renamed} — curToken is `{`.
func (p *Parser) parseHashPattern() ast.Expression {
	pat := &ast.HashPattern{Token: p.curToken}
	if p.peekTokenIs(lexer.RBRACE) {
		p.nextToken() // empty {}
		return pat
	}
	p.nextToken() // first key
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		if !p.curTokenIs(lexer.IDENT) {
			p.errors = append(p.errors, "object destructure pattern expects an identifier key")
			return nil
		}
		key := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		value := key
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // :
			if !p.expectPeek(lexer.IDENT) {
				return nil
			}
			value = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		}
		pat.Entries = append(pat.Entries, ast.HashPatternEntry{Key: key, Value: value})
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			continue
		}
		break
	}
	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}
	return pat
}

// ---- Wave 1: spread ----

// parseSpreadExpression: ...expr — curToken is `...`.
func (p *Parser) parseSpreadExpression() ast.Expression {
	expr := &ast.SpreadExpression{Token: p.curToken}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	return expr
}

// ---- Wave 1: optional chaining ----

// parseOptionalChainExpression: a?.b — curToken is `?.`.
// Greedily folds following .ident / [expr] / (args) / ?. links into one node.
func (p *Parser) parseOptionalChainExpression(left ast.Expression) ast.Expression {
	expr := &ast.OptionalChainExpression{Token: p.curToken, Base: left}
	p.nextToken() // move to first link
	for {
		link := p.parseChainLink()
		if link == nil {
			return nil
		}
		expr.Links = append(expr.Links, *link)
		switch {
		case p.peekTokenIs(lexer.DOT):
			p.nextToken() // .
			p.nextToken() // link start
		case p.peekTokenIs(lexer.LBRACKET):
			p.nextToken() // [
		case p.peekTokenIs(lexer.LPAREN):
			p.nextToken() // (
		case p.peekTokenIs(lexer.OPTIONAL_CHAIN):
			p.nextToken() // ?.
			p.nextToken() // link start
		default:
			return expr
		}
	}
}

// parseChainLink parses one link; curToken is the link's first token.
func (p *Parser) parseChainLink() *ast.ChainLink {
	switch {
	case p.curTokenIs(lexer.IDENT):
		return &ast.ChainLink{
			Kind:     ast.ChainMember,
			Property: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
		}
	case p.curTokenIs(lexer.LBRACKET):
		p.nextToken()
		idx := p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		return &ast.ChainLink{Kind: ast.ChainIndex, Index: idx}
	case p.curTokenIs(lexer.LPAREN):
		args := p.parseCallArguments()
		return &ast.ChainLink{Kind: ast.ChainCall, Arguments: args}
	default:
		p.errors = append(p.errors, "expected property, [index], or (args) after ?.")
		return nil
	}
}

// ---- Wave 1: pipeline ----

// parsePipelineExpression: x |> f |> g — desugars to nested CallExpressions.
// curToken is `|>`.
func (p *Parser) parsePipelineExpression(left ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken()
	fn := p.parseExpression(PIPELINE)
	if fn == nil {
		return nil
	}
	if call, ok := fn.(*ast.CallExpression); ok {
		call.Arguments = append([]ast.Expression{left}, call.Arguments...)
		return call
	}
	return &ast.CallExpression{Token: tok, Function: fn, Arguments: []ast.Expression{left}}
}

// ---- Wave 1: ranges ----

// parseDotInfix handles both member access (a.b) and inclusive ranges (1..10).
func (p *Parser) parseDotInfix(left ast.Expression) ast.Expression {
	if p.peekTokenIs(lexer.DOT) {
		return p.parseInclusiveRangeExpression(left)
	}
	return p.parseMemberExpression(left)
}

// parseDColonInfix handles `::` — wave 19:
//   - `ffi::call("x", [])`: namespace access, parsed exactly like `.`
//     member access (zero evaluator changes; `ffi` simply isn't defined).
//   - `v.map::<string>(f)`: turbofish explicit type arguments on a call.
//     Bare `v.map<string>(f)` is rejected with a helpful message (the `<`
//     stays less-than there); plain `f<string>(x)` keeps working via the
//     probe in parseExpression.
func (p *Parser) parseDColonInfix(left ast.Expression) ast.Expression {
	tok := p.curToken
	if p.peekTokenIs(lexer.LT) {
		targs := p.parseTypeArgList()
		if targs == nil {
			return nil
		}
		if !p.expectPeek(lexer.LPAREN) {
			p.errors = append(p.errors, "turbofish type arguments must be followed by a call: v.map::<T>(...)")
			return nil
		}
		call := &ast.CallExpression{Token: tok, Function: left, TypeArgs: targs}
		call.Arguments = p.parseCallArguments()
		return call
	}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	return &ast.MemberExpression{
		Token:    tok,
		Object:   left,
		Property: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
	}
}

// parsePostfixExpression handles `x++` / `x--` — wave 19.
func (p *Parser) parsePostfixExpression(left ast.Expression) ast.Expression {
	return &ast.PostfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Operand:  left,
	}
}

// parseInclusiveRangeExpression: 1..10 — curToken is the first DOT.
func (p *Parser) parseInclusiveRangeExpression(left ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken() // consume second DOT
	p.nextToken() // move to start of end expression
	end := p.parseExpression(RANGE)
	return &ast.RangeExpression{Token: tok, Start: left, End: end, Inclusive: true}
}

// parseExclusiveRangeExpression: 1...5 — curToken is `...`.
func (p *Parser) parseExclusiveRangeExpression(left ast.Expression) ast.Expression {
	tok := p.curToken
	p.nextToken() // move to start of end expression
	end := p.parseExpression(RANGE)
	return &ast.RangeExpression{Token: tok, Start: left, End: end, Inclusive: false}
}

// ---- Wave 1: loop else ----

// parseLoopElse parses a trailing `else { ... }` after a loop body block.
// curToken is the loop body's closing `}`.
func (p *Parser) parseLoopElse() *ast.BlockStatement {
	if !p.peekTokenIs(lexer.ELSE) {
		return nil
	}
	p.nextToken() // else
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	return p.parseBlockStatement()
}

// ---- Wave 19: generics ----

// parseTypeParamList parses `<T, U: Bound, V>` — a type *parameter* list.
// Call with peek == LT. On success curToken is the closing `>` (a `>>`
// closing two levels is consumed and recorded via pendingCloses).
func (p *Parser) parseTypeParamList() []*ast.TypeParam {
	p.nextToken() // consume `<`
	var params []*ast.TypeParam
	for {
		p.nextToken() // first token of the parameter name
		if !p.curTokenIs(lexer.IDENT) {
			p.errors = append(p.errors, "expected type parameter name")
			return nil
		}
		tp := &ast.TypeParam{
			Token: p.curToken,
			Name:  &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
		}
		if p.peekTokenIs(lexer.COLON) {
			p.nextToken() // :
			p.nextToken() // first token of the bound
			tp.Bound = p.parseTypeAnnotation()
			if tp.Bound == nil {
				return nil
			}
			// A nested `>>` inside the bound may have eaten our close.
			if p.pendingCloses > 0 {
				p.pendingCloses--
				params = append(params, tp)
				return params
			}
		}
		params = append(params, tp)
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			continue
		}
		if p.peekTokenIs(lexer.GT) {
			p.nextToken()
			return params
		}
		if p.peekTokenIs(lexer.SHR) {
			// `>>` closes this list and the enclosing one.
			p.nextToken()
			p.pendingCloses++
			return params
		}
		p.errors = append(p.errors, "expected ',' or '>' in type parameter list")
		return nil
	}
}

// parseTypeArgList parses `<A, B<C>, D>` — a type *argument* list.
// Call with peek == LT. On success curToken is the closing `>`.
func (p *Parser) parseTypeArgList() []*ast.TypeAnnotation {
	p.nextToken() // consume `<`
	var args []*ast.TypeAnnotation
	for {
		p.nextToken() // first token of the argument
		arg := p.parseTypeAnnotation()
		if arg == nil {
			return nil
		}
		args = append(args, arg)
		// A nested `>>` inside the argument may have eaten our close.
		if p.pendingCloses > 0 {
			p.pendingCloses--
			return args
		}
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			continue
		}
		if p.peekTokenIs(lexer.GT) {
			p.nextToken()
			return args
		}
		if p.peekTokenIs(lexer.SHR) {
			// `>>` closes this list and the enclosing one.
			p.nextToken()
			p.pendingCloses++
			return args
		}
		p.errors = append(p.errors, "expected ',' or '>' in type argument list")
		return nil
	}
}

// parseReturnAnnotation parses an optional `: T` / `-> T` return annotation.
// Call with curToken on the token before the annotation (`)` of the params).
// Returns nil when no annotation is present.
func (p *Parser) parseReturnAnnotation() *ast.TypeAnnotation {
	if !p.peekTokenIs(lexer.COLON) && !p.peekTokenIs(lexer.ARROW) {
		return nil
	}
	p.nextToken() // : or ->
	p.nextToken() // first token of the type
	ann := p.parseTypeAnnotation()
	return ann
}

// ---- Wave 1: string interpolation ----

// parseStringLiteral with ${...} interpolation support.
func (p *Parser) parseStringLiteral() ast.Expression {
	tok := p.curToken
	parts, err := splitInterpolation(tok.Literal)
	if err != nil {
		p.errors = append(p.errors, err.Error())
		return nil
	}
	// Fast path: no interpolation.
	if len(parts) == 1 && !parts[0].isExpr {
		return &ast.StringLiteral{Token: tok, Value: parts[0].text}
	}
	node := &ast.InterpolatedString{Token: tok}
	for _, part := range parts {
		if !part.isExpr {
			node.Parts = append(node.Parts, &ast.StringLiteral{Token: tok, Value: part.text})
			continue
		}
		sub, err := parseInterpSubExpression(part.text)
		if err != nil {
			p.errors = append(p.errors, "bad ${...}: "+err.Error())
			return nil
		}
		node.Parts = append(node.Parts, sub)
	}
	return node
}

type interpPart struct {
	isExpr bool
	text   string
}

// splitInterpolation splits s on ${...} with balanced-brace matching.
// String literals inside the braces are respected so `${m["}"]}` works.
func splitInterpolation(s string) ([]interpPart, error) {
	var parts []interpPart
	var lit strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end, err := findInterpEnd(s, i+2)
			if err != nil {
				return nil, err
			}
			if lit.Len() > 0 {
				parts = append(parts, interpPart{isExpr: false, text: lit.String()})
				lit.Reset()
			}
			parts = append(parts, interpPart{isExpr: true, text: s[i+2 : end]})
			i = end + 1
		} else {
			lit.WriteByte(s[i])
			i++
		}
	}
	if lit.Len() > 0 {
		parts = append(parts, interpPart{isExpr: false, text: lit.String()})
	}
	if len(parts) == 0 {
		parts = append(parts, interpPart{isExpr: false, text: ""})
	}
	return parts, nil
}

// findInterpEnd finds the index of the `}` closing the `${` at start-2.
func findInterpEnd(s string, start int) (int, error) {
	depth := 1
	var quote byte
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated ${ in string")
}

// parseInterpSubExpression parses the text inside ${...} with a fresh parser.
func parseInterpSubExpression(text string) (ast.Expression, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty expression")
	}
	l := lexer.New(text)
	sub := New(l)
	prog := sub.ParseProgram()
	if len(sub.Errors()) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(sub.Errors(), "; "))
	}
	if len(prog.Statements) != 1 {
		return nil, fmt.Errorf("expected a single expression")
	}
	es, ok := prog.Statements[0].(*ast.ExpressionStatement)
	if !ok || es.Expression == nil {
		return nil, fmt.Errorf("expected a single expression")
	}
	return es.Expression, nil
}

// Helpers

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) peekError(t lexer.TokenType) {
	// Wave 21: errors carry line:col and read like a human wrote them.
	tok := p.peekToken
	found := describeToken(tok.Type, tok.Literal)
	p.errors = append(p.errors, fmt.Sprintf("line %d:%d: expected %s, found %s",
		tok.Line, tok.Column, describeToken(t, ""), found))
}

func (p *Parser) noPrefixParseFnError(t lexer.TokenType) {
	// Wave 21: point at the offending token, not just its type.
	tok := p.curToken
	p.errors = append(p.errors, fmt.Sprintf("line %d:%d: unexpected %s",
		tok.Line, tok.Column, describeToken(tok.Type, tok.Literal)))
}

// describeToken renders a token type for error messages: punctuation as
// 'x', identifiers/numbers/strings with their literal, EOF as end of input.
func describeToken(t lexer.TokenType, lit string) string {
	switch t {
	case lexer.IDENT:
		if lit != "" {
			return fmt.Sprintf("identifier %q", lit)
		}
		return "identifier"
	case lexer.INT, lexer.FLOAT:
		if lit != "" {
			return fmt.Sprintf("number %q", lit)
		}
		return "number"
	case lexer.STRING:
		if lit != "" {
			return fmt.Sprintf("string %q", lit)
		}
		return "string"
	case lexer.EOF:
		return "end of input"
	case lexer.ILLEGAL:
		if lit != "" {
			return fmt.Sprintf("illegal character %q", lit)
		}
		return "illegal character"
	default:
		return fmt.Sprintf("'%s'", string(t))
	}
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) registerPrefix(tokenType lexer.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType lexer.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) parseEnumStatement() *ast.EnumStatement {
	stmt := &ast.EnumStatement{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	stmt.Members = []*ast.Identifier{}
	if !p.peekTokenIs(lexer.RBRACE) {
		p.nextToken()
		if p.curTokenIs(lexer.IDENT) {
			stmt.Members = append(stmt.Members, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
		}
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			if p.curTokenIs(lexer.IDENT) {
				stmt.Members = append(stmt.Members, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			}
		}
	}
	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}
	return stmt
}

// parseRecordStatement: record Point(x, y) — wave 3.
func (p *Parser) parseRecordStatement() ast.Statement {
	stmt := &ast.RecordStatement{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	stmt.Fields = []*ast.Identifier{}
	seen := map[string]bool{}
	addField := func() bool {
		if !p.curTokenIs(lexer.IDENT) {
			p.peekError(lexer.IDENT)
			return false
		}
		name := p.curToken.Literal
		if seen[name] {
			p.errors = append(p.errors, fmt.Sprintf("duplicate field %q in record %s", name, stmt.Name.Value))
			return false
		}
		seen[name] = true
		stmt.Fields = append(stmt.Fields, &ast.Identifier{Token: p.curToken, Value: name})
		return true
	}
	if !p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		if !addField() {
			return nil
		}
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			if !addField() {
				return nil
			}
		}
	}
	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	stmt := &ast.DeferStatement{Token: p.curToken}
	p.nextToken()
	// Allow: defer print expr   as well as defer call(...)
	if p.curTokenIs(lexer.PRINT) {
		p.nextToken()
		arg := p.parseExpression(LOWEST)
		// desugar to call of builtin print via identifier
		stmt.Call = &ast.CallExpression{
			Token:     stmt.Token,
			Function:  &ast.Identifier{Token: stmt.Token, Value: "print"},
			Arguments: []ast.Expression{arg},
		}
	} else {
		stmt.Call = p.parseExpression(LOWEST)
	}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}
