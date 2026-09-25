package ast

import (
	"bytes"
	"strings"

	"github.com/navescript/nvs/internal/lexer"
)

// Node is the base interface for all AST nodes.
type Node interface {
	TokenLiteral() string
	String() string
}

// Statement nodes produce no value (or side effects only).
type Statement interface {
	Node
	statementNode()
}

// Expression nodes produce a value.
type Expression interface {
	Node
	expressionNode()
}

// Program is the root of every AST.
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// LetStatement: let x = expr;
type LetStatement struct {
	Token lexer.Token
	Name  *Identifier
	Value Expression
}

func (ls *LetStatement) statementNode()       {}
func (ls *LetStatement) TokenLiteral() string { return ls.Token.Literal }
func (ls *LetStatement) String() string {
	var out bytes.Buffer
	out.WriteString("let ")
	out.WriteString(ls.Name.String())
	out.WriteString(" = ")
	if ls.Value != nil {
		out.WriteString(ls.Value.String())
	}
	out.WriteString(";")
	return out.String()
}

// ReturnStatement: return expr;
type ReturnStatement struct {
	Token       lexer.Token
	ReturnValue Expression
}

func (rs *ReturnStatement) statementNode()       {}
func (rs *ReturnStatement) TokenLiteral() string { return rs.Token.Literal }
func (rs *ReturnStatement) String() string {
	var out bytes.Buffer
	out.WriteString("return ")
	if rs.ReturnValue != nil {
		out.WriteString(rs.ReturnValue.String())
	}
	out.WriteString(";")
	return out.String()
}

// ExpressionStatement wraps an expression used as a statement.
type ExpressionStatement struct {
	Token      lexer.Token
	Expression Expression
}

func (es *ExpressionStatement) statementNode()       {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// BlockStatement: { stmt; stmt; ... }
type BlockStatement struct {
	Token      lexer.Token
	Statements []Statement
}

func (bs *BlockStatement) statementNode()       {}
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	out.WriteString("{ ")
	for _, s := range bs.Statements {
		out.WriteString(s.String())
	}
	out.WriteString(" }")
	return out.String()
}

// Identifier
type Identifier struct {
	Token lexer.Token
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// IntegerLiteral
type IntegerLiteral struct {
	Token lexer.Token
	Value int64
}

func (il *IntegerLiteral) expressionNode()      {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return il.Token.Literal }

// FloatLiteral
type FloatLiteral struct {
	Token lexer.Token
	Value float64
}

func (fl *FloatLiteral) expressionNode()      {}
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return fl.Token.Literal }

// StringLiteral
type StringLiteral struct {
	Token lexer.Token
	Value string
}

func (sl *StringLiteral) expressionNode()      {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string       { return `"` + sl.Value + `"` }

// Boolean
type Boolean struct {
	Token lexer.Token
	Value bool
}

func (b *Boolean) expressionNode()      {}
func (b *Boolean) TokenLiteral() string { return b.Token.Literal }
func (b *Boolean) String() string       { return b.Token.Literal }

// PrefixExpression: !x, -x
type PrefixExpression struct {
	Token    lexer.Token
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	return "(" + pe.Operator + pe.Right.String() + ")"
}

// InfixExpression: a + b, a == b, etc.
type InfixExpression struct {
	Token    lexer.Token
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode()      {}
func (ie *InfixExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *InfixExpression) String() string {
	return "(" + ie.Left.String() + " " + ie.Operator + " " + ie.Right.String() + ")"
}

// IfExpression: if (cond) { ... } else { ... }
type IfExpression struct {
	Token       lexer.Token
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (ie *IfExpression) expressionNode()      {}
func (ie *IfExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IfExpression) String() string {
	var out bytes.Buffer
	out.WriteString("if ")
	out.WriteString(ie.Condition.String())
	out.WriteString(" ")
	out.WriteString(ie.Consequence.String())
	if ie.Alternative != nil {
		out.WriteString(" else ")
		out.WriteString(ie.Alternative.String())
	}
	return out.String()
}

// WhileExpression / statement
type WhileStatement struct {
	Token     lexer.Token
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode()       {}
func (ws *WhileStatement) TokenLiteral() string { return ws.Token.Literal }
func (ws *WhileStatement) String() string {
	return "while " + ws.Condition.String() + " " + ws.Body.String()
}

// FunctionLiteral: fn(x, y) { ... }  (optional defaults: fn(x, y=1))
type FunctionLiteral struct {
	Token      lexer.Token
	Parameters []*Identifier
	Defaults   []Expression // parallel to Parameters; nil entry = required
	Body       *BlockStatement
}

func (fl *FunctionLiteral) expressionNode()      {}
func (fl *FunctionLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FunctionLiteral) String() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range fl.Parameters {
		params = append(params, p.String())
	}
	out.WriteString("fn(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") ")
	out.WriteString(fl.Body.String())
	return out.String()
}

// CallExpression: f(a, b)
type CallExpression struct {
	Token     lexer.Token
	Function  Expression
	Arguments []Expression
}

func (ce *CallExpression) expressionNode()      {}
func (ce *CallExpression) TokenLiteral() string { return ce.Token.Literal }
func (ce *CallExpression) String() string {
	var out bytes.Buffer
	args := []string{}
	for _, a := range ce.Arguments {
		args = append(args, a.String())
	}
	out.WriteString(ce.Function.String())
	out.WriteString("(")
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")")
	return out.String()
}

// PrintStatement: print expr;
type PrintStatement struct {
	Token lexer.Token
	Value Expression
}

func (ps *PrintStatement) statementNode()       {}
func (ps *PrintStatement) TokenLiteral() string { return ps.Token.Literal }
func (ps *PrintStatement) String() string {
	return "print " + ps.Value.String() + ";"
}

// ArrayLiteral: [1, 2, 3]
type ArrayLiteral struct {
	Token    lexer.Token
	Elements []Expression
}

func (al *ArrayLiteral) expressionNode()      {}
func (al *ArrayLiteral) TokenLiteral() string { return al.Token.Literal }
func (al *ArrayLiteral) String() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range al.Elements {
		elements = append(elements, e.String())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// IndexExpression: arr[0] or slice arr[1:3] (End != nil means slice)
type IndexExpression struct {
	Token lexer.Token
	Left  Expression
	Index Expression // start for slice; single index otherwise
	End   Expression // if non-nil, this is a slice [Index:End]
}

func (ie *IndexExpression) expressionNode()      {}
func (ie *IndexExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *IndexExpression) String() string {
	return "(" + ie.Left.String() + "[" + ie.Index.String() + "])"
}

// AssignExpression: x = expr  (also used as statement)
type AssignExpression struct {
	Token lexer.Token
	Name  *Identifier
	Value Expression
}

func (ae *AssignExpression) expressionNode()      {}
func (ae *AssignExpression) TokenLiteral() string { return ae.Token.Literal }
func (ae *AssignExpression) String() string {
	return ae.Name.String() + " = " + ae.Value.String()
}

// ForStatement: for (init; condition; post) { body }
// init and post may be nil
type ForStatement struct {
	Token     lexer.Token
	Init      Statement   // let x = 0  or  x = 0  or expression statement
	Condition Expression
	Post      Expression  // typically assignment or call
	Body      *BlockStatement
}

func (fs *ForStatement) statementNode()       {}
func (fs *ForStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for (")
	if fs.Init != nil {
		out.WriteString(fs.Init.String())
	}
	out.WriteString("; ")
	if fs.Condition != nil {
		out.WriteString(fs.Condition.String())
	}
	out.WriteString("; ")
	if fs.Post != nil {
		out.WriteString(fs.Post.String())
	}
	out.WriteString(") ")
	out.WriteString(fs.Body.String())
	return out.String()
}

// BreakStatement
type BreakStatement struct {
	Token lexer.Token
}

func (bs *BreakStatement) statementNode()       {}
func (bs *BreakStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BreakStatement) String() string       { return "break;" }

// ContinueStatement
type ContinueStatement struct {
	Token lexer.Token
}

func (cs *ContinueStatement) statementNode()       {}
func (cs *ContinueStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ContinueStatement) String() string       { return "continue;" }

// HashLiteral: { "key": value, ... }
type HashLiteral struct {
	Token lexer.Token
	Pairs map[Expression]Expression
}

func (hl *HashLiteral) expressionNode()      {}
func (hl *HashLiteral) TokenLiteral() string { return hl.Token.Literal }
func (hl *HashLiteral) String() string {
	var out bytes.Buffer
	pairs := []string{}
	for k, v := range hl.Pairs {
		pairs = append(pairs, k.String()+": "+v.String())
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// ForInStatement: for (x in arr) { ... }
type ForInStatement struct {
	Token    lexer.Token
	Name     *Identifier
	Iterable Expression
	Body     *BlockStatement
}

func (fs *ForInStatement) statementNode()       {}
func (fs *ForInStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForInStatement) String() string {
	return "for (" + fs.Name.String() + " in " + fs.Iterable.String() + ") " + fs.Body.String()
}

// ImportStatement: import "path.ns"
type ImportStatement struct {
	Token lexer.Token
	Path  *StringLiteral
}

func (is *ImportStatement) statementNode()       {}
func (is *ImportStatement) TokenLiteral() string { return is.Token.Literal }
func (is *ImportStatement) String() string {
	return "import " + is.Path.String()
}

// IndexAssignExpression: arr[i] = value  or  map[key] = value
type IndexAssignExpression struct {
	Token lexer.Token
	Left  *IndexExpression
	Value Expression
}

func (ia *IndexAssignExpression) expressionNode()      {}
func (ia *IndexAssignExpression) TokenLiteral() string { return ia.Token.Literal }
func (ia *IndexAssignExpression) String() string {
	return ia.Left.String() + " = " + ia.Value.String()
}

// ClassStatement: class Name { methods... }  or  class Name extends Parent { ... }
type ClassStatement struct {
	Token      lexer.Token
	Name       *Identifier
	Parent     *Identifier // optional
	Methods    []*ClassMethod
}

type ClassMethod struct {
	Token      lexer.Token
	Name       *Identifier
	Parameters []*Identifier
	Body       *BlockStatement
}

func (cs *ClassStatement) statementNode()       {}
func (cs *ClassStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ClassStatement) String() string {
	var out bytes.Buffer
	out.WriteString("class ")
	out.WriteString(cs.Name.String())
	if cs.Parent != nil {
		out.WriteString(" extends ")
		out.WriteString(cs.Parent.String())
	}
	out.WriteString(" { ... }")
	return out.String()
}

// NewExpression: new ClassName(args)
type NewExpression struct {
	Token     lexer.Token
	ClassName *Identifier
	Arguments []Expression
}

func (ne *NewExpression) expressionNode()      {}
func (ne *NewExpression) TokenLiteral() string { return ne.Token.Literal }
func (ne *NewExpression) String() string {
	args := []string{}
	for _, a := range ne.Arguments {
		args = append(args, a.String())
	}
	return "new " + ne.ClassName.String() + "(" + strings.Join(args, ", ") + ")"
}

// ThisExpression: this
type ThisExpression struct {
	Token lexer.Token
}

func (te *ThisExpression) expressionNode()      {}
func (te *ThisExpression) TokenLiteral() string { return te.Token.Literal }
func (te *ThisExpression) String() string       { return "this" }

// MemberExpression: obj.field  or  obj.method
type MemberExpression struct {
	Token    lexer.Token
	Object   Expression
	Property *Identifier
}

func (me *MemberExpression) expressionNode()      {}
func (me *MemberExpression) TokenLiteral() string { return me.Token.Literal }
func (me *MemberExpression) String() string {
	return me.Object.String() + "." + me.Property.String()
}

// MemberAssignExpression: obj.field = value
type MemberAssignExpression struct {
	Token    lexer.Token
	Object   Expression
	Property *Identifier
	Value    Expression
}

func (ma *MemberAssignExpression) expressionNode()      {}
func (ma *MemberAssignExpression) TokenLiteral() string { return ma.Token.Literal }
func (ma *MemberAssignExpression) String() string {
	return ma.Object.String() + "." + ma.Property.String() + " = " + ma.Value.String()
}

// TryStatement: try { ... } catch (e) { ... } finally { ... }
type TryStatement struct {
	Token   lexer.Token
	Body    *BlockStatement
	Catch   *BlockStatement
	CatchId *Identifier // optional binding
	Finally *BlockStatement
}

func (ts *TryStatement) statementNode()       {}
func (ts *TryStatement) TokenLiteral() string { return ts.Token.Literal }
func (ts *TryStatement) String() string       { return "try { ... } catch { ... }" }

// ThrowStatement: throw expr
type ThrowStatement struct {
	Token lexer.Token
	Value Expression
}

func (ts *ThrowStatement) statementNode()       {}
func (ts *ThrowStatement) TokenLiteral() string { return ts.Token.Literal }
func (ts *ThrowStatement) String() string {
	return "throw " + ts.Value.String()
}

// MatchExpression: match (expr) { case pat: body; default: body }
type MatchExpression struct {
	Token   lexer.Token
	Value   Expression
	Arms    []*MatchArm
	Default *BlockStatement
}

type MatchArm struct {
	Pattern Expression // literal or identifier (wildcard-ish)
	Body    *BlockStatement
}

func (me *MatchExpression) expressionNode()      {}
func (me *MatchExpression) TokenLiteral() string { return me.Token.Literal }
func (me *MatchExpression) String() string       { return "match (...) { ... }" }

// ConstStatement: const x = expr (non-reassignable)
type ConstStatement struct {
	Token lexer.Token
	Name  *Identifier
	Value Expression
}

func (cs *ConstStatement) statementNode()       {}
func (cs *ConstStatement) TokenLiteral() string { return cs.Token.Literal }
func (cs *ConstStatement) String() string {
	return "const " + cs.Name.String() + " = " + cs.Value.String()
}

// YieldStatement: yield expr
type YieldStatement struct {
	Token lexer.Token
	Value Expression
}

func (ys *YieldStatement) statementNode()       {}
func (ys *YieldStatement) TokenLiteral() string { return ys.Token.Literal }
func (ys *YieldStatement) String() string {
	if ys.Value != nil {
		return "yield " + ys.Value.String()
	}
	return "yield"
}

// DecoratorExpression applied to next function statement
// @decorator
// fn name() { }
type DecoratorStatement struct {
	Token      lexer.Token
	Decorator  *Identifier
	Function   Statement // LetStatement binding a function
}

func (ds *DecoratorStatement) statementNode()       {}
func (ds *DecoratorStatement) TokenLiteral() string { return ds.Token.Literal }
func (ds *DecoratorStatement) String() string {
	return "@" + ds.Decorator.String() + " " + ds.Function.String()
}

// TypeAnnotation optional: let x: int = 1  (stored for future checker)
type TypedLetStatement struct {
	Token    lexer.Token
	Name     *Identifier
	TypeName *Identifier
	Value    Expression
}

func (tl *TypedLetStatement) statementNode()       {}
func (tl *TypedLetStatement) TokenLiteral() string { return tl.Token.Literal }
func (tl *TypedLetStatement) String() string {
	return "let " + tl.Name.String() + ": " + tl.TypeName.String() + " = " + tl.Value.String()
}

// TernaryExpression: cond ? a : b
type TernaryExpression struct {
	Token       lexer.Token
	Condition   Expression
	Consequence Expression
	Alternative Expression
}

func (te *TernaryExpression) expressionNode()      {}
func (te *TernaryExpression) TokenLiteral() string { return te.Token.Literal }
func (te *TernaryExpression) String() string {
	return "(" + te.Condition.String() + " ? " + te.Consequence.String() + " : " + te.Alternative.String() + ")"
}

// NullLiteral: null
type NullLiteral struct {
	Token lexer.Token
}

func (nl *NullLiteral) expressionNode()      {}
func (nl *NullLiteral) TokenLiteral() string { return nl.Token.Literal }
func (nl *NullLiteral) String() string       { return "null" }


// EnumStatement: enum Color { Red, Green, Blue }
type EnumStatement struct {
	Token  lexer.Token
	Name   *Identifier
	Members []*Identifier
}

func (es *EnumStatement) statementNode()       {}
func (es *EnumStatement) TokenLiteral() string { return es.Token.Literal }
func (es *EnumStatement) String() string       { return "enum " + es.Name.String() }

// DeferStatement: defer expr/call  (runs at end of enclosing function/block)
type DeferStatement struct {
	Token lexer.Token
	Call  Expression
}

func (ds *DeferStatement) statementNode()       {}
func (ds *DeferStatement) TokenLiteral() string { return ds.Token.Literal }
func (ds *DeferStatement) String() string       { return "defer ..." }

// SpreadElement used inside arrays: [...arr]
type SpreadExpression struct {
	Token lexer.Token
	Value Expression
}

func (se *SpreadExpression) expressionNode()      {}
func (se *SpreadExpression) TokenLiteral() string { return se.Token.Literal }
func (se *SpreadExpression) String() string       { return "..." + se.Value.String() }
