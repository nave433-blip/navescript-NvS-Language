package parser

import (
	"fmt"
	"strconv"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

const (
	_ int = iota
	LOWEST
	ASSIGN_PREC // =
	EQUALS      // ==
	LESSGREATER // > or <
	SUM         // +
	PRODUCT     // *
	PREFIX      // -X or !X
	CALL        // myFunction(X)
	INDEX       // array[index]
)

var precedences = map[lexer.TokenType]int{
	lexer.ASSIGN:   ASSIGN_PREC,
	lexer.PLUS_ASSIGN: ASSIGN_PREC,
	lexer.MINUS_ASSIGN: ASSIGN_PREC,
	lexer.STAR_ASSIGN: ASSIGN_PREC,
	lexer.SLASH_ASSIGN: ASSIGN_PREC,
	lexer.QUESTION:  ASSIGN_PREC,
	lexer.NULL_COAL: ASSIGN_PREC,
	lexer.IN:        EQUALS,
	lexer.EQ:       EQUALS,
	lexer.NOT_EQ:   EQUALS,
	lexer.LT:       LESSGREATER,
	lexer.GT:       LESSGREATER,
	lexer.LTE:      LESSGREATER,
	lexer.GTE:      LESSGREATER,
	lexer.PLUS:     SUM,
	lexer.MINUS:    SUM,
	lexer.SLASH:    PRODUCT,
	lexer.ASTERISK: PRODUCT,
	lexer.MOD:      PRODUCT,
	lexer.BIT_AND:  PRODUCT,
	lexer.BIT_OR:   PRODUCT,
	lexer.BIT_XOR:  PRODUCT,
	lexer.SHL:      PRODUCT,
	lexer.SHR:      PRODUCT,
	lexer.LPAREN:   CALL,
	lexer.LBRACKET: INDEX,
	lexer.DOT:     INDEX,
	lexer.AND:      EQUALS,
	lexer.OR:       EQUALS,
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

	p.infixParseFns = make(map[lexer.TokenType]infixParseFn)
	p.registerInfix(lexer.PLUS, p.parseInfixExpression)
	p.registerInfix(lexer.MINUS, p.parseInfixExpression)
	p.registerInfix(lexer.SLASH, p.parseInfixExpression)
	p.registerInfix(lexer.ASTERISK, p.parseInfixExpression)
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
	p.registerInfix(lexer.DOT, p.parseMemberExpression)

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
	case lexer.DEFER:
		return p.parseDeferStatement()
	case lexer.THROW:
		return p.parseThrowStatement()
	case lexer.TRY:
		return p.parseTryStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseLetStatement() ast.Statement {
	tok := p.curToken

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}

	name := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	var typeName *ast.Identifier
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // :
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		typeName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	if !p.expectPeek(lexer.ASSIGN) {
		return nil
	}

	p.nextToken()
	value := p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	if typeName != nil {
		return &ast.TypedLetStatement{Token: tok, Name: name, TypeName: typeName, Value: value}
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
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
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

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	params, defaults := p.parseFunctionParameters()

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	body := p.parseBlockStatement()

	fn := &ast.FunctionLiteral{
		Token:      tok,
		Parameters: params,
		Defaults:   defaults,
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

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
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
	p.nextToken()
	exp := p.parseExpression(LOWEST)
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
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	expression.Consequence = p.parseBlockStatement()

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
			if !p.expectPeek(lexer.LBRACE) {
				return nil
			}
			expression.Alternative = p.parseBlockStatement()
		}
	}

	return expression
}

func (p *Parser) parseFunctionLiteral() ast.Expression {
	lit := &ast.FunctionLiteral{Token: p.curToken}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	lit.Parameters, lit.Defaults = p.parseFunctionParameters()

	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()
	return lit
}

func (p *Parser) parseFunctionParameters() ([]*ast.Identifier, []ast.Expression) {
	identifiers := []*ast.Identifier{}
	defaults := []ast.Expression{}

	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return identifiers, defaults
	}

	p.nextToken()
	ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	identifiers = append(identifiers, ident)
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
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken()
			p.nextToken()
			defaults = append(defaults, p.parseExpression(LOWEST))
		} else {
			defaults = append(defaults, nil)
		}
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil, nil
	}

	return identifiers, defaults
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
			if !p.expectPeek(lexer.LBRACE) {
				return nil
			}
			body := p.parseBlockStatement()
			return &ast.ForInStatement{
				Token:    tok,
				Name:     name,
				Iterable: iterable,
				Body:     body,
			}
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
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
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

func (p *Parser) parseConstStatement() *ast.ConstStatement {
	stmt := &ast.ConstStatement{Token: p.curToken}
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
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	if !p.expectPeek(lexer.RPAREN) {
		return nil
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
			arm.Pattern = p.parseExpression(LOWEST)
			// optional colon
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
			}
			if !p.expectPeek(lexer.LBRACE) {
				// single expression arm: case 1: print x
				// treat rest as expression statement block
				stmt := p.parseStatement()
				arm.Body = &ast.BlockStatement{
					Statements: []ast.Statement{stmt},
				}
			} else {
				arm.Body = p.parseBlockStatement()
			}
			expr.Arms = append(expr.Arms, arm)
			p.nextToken()
		} else if p.curTokenIs(lexer.DEFAULT) {
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
			}
			if !p.expectPeek(lexer.LBRACE) {
				return nil
			}
			expr.Default = p.parseBlockStatement()
			p.nextToken()
		} else {
			p.nextToken()
		}
	}
	return expr
}

func (p *Parser) parseClassStatement() *ast.ClassStatement {
	stmt := &ast.ClassStatement{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

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
	p.nextToken()
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		// method: name(params) { body }  or  fn name(params) { body }
		if p.curTokenIs(lexer.FUNCTION) {
			p.nextToken()
		}
		if !p.curTokenIs(lexer.IDENT) {
			p.nextToken()
			continue
		}
		method := &ast.ClassMethod{Token: p.curToken}
		method.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if !p.expectPeek(lexer.LPAREN) {
			return nil
		}
		method.Parameters, _ = p.parseFunctionParameters()
		if !p.expectPeek(lexer.LBRACE) {
			return nil
		}
		method.Body = p.parseBlockStatement()
		stmt.Methods = append(stmt.Methods, method)
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseNewExpression() ast.Expression {
	expr := &ast.NewExpression{Token: p.curToken}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	expr.ClassName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}
	expr.Arguments = p.parseCallArguments()
	return expr
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
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseContinueStatement() *ast.ContinueStatement {
	stmt := &ast.ContinueStatement{Token: p.curToken}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseImportStatement() *ast.ImportStatement {
	stmt := &ast.ImportStatement{Token: p.curToken}
	if !p.expectPeek(lexer.STRING) {
		return nil
	}
	stmt.Path = &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseHashLiteral() ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken}
	hash.Pairs = make(map[ast.Expression]ast.Expression)

	for !p.peekTokenIs(lexer.RBRACE) {
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

	p.nextToken()
	args = append(args, p.parseExpression(LOWEST))

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()
		args = append(args, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	return args
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
	msg := fmt.Sprintf("expected next token to be %s, got %s instead", t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) noPrefixParseFnError(t lexer.TokenType) {
	msg := fmt.Sprintf("no prefix parse function for %s found", t)
	p.errors = append(p.errors, msg)
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

func (p *Parser) parseDeferStatement() *ast.DeferStatement {
	stmt := &ast.DeferStatement{Token: p.curToken}
	p.nextToken()
	// Allow: defer print expr   as well as defer call(...)
	if p.curTokenIs(lexer.PRINT) {
		p.nextToken()
		arg := p.parseExpression(LOWEST)
		// desugar to call of builtin print via identifier
		stmt.Call = &ast.CallExpression{
			Token:    stmt.Token,
			Function: &ast.Identifier{Token: stmt.Token, Value: "print"},
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
