package bytecode

import (
	"fmt"

	"github.com/navescript/nvs/internal/ast"
)

type Bytecode struct {
	Instructions []byte
	Constants    []interface{}
	Names        []string // global names by index
}

type Compiler struct {
	instructions []byte
	constants    []interface{}
	names        []string
	nameIndex    map[string]int
}

func NewCompiler() *Compiler {
	return &Compiler{
		nameIndex: map[string]int{},
	}
}

func (c *Compiler) Bytecode() *Bytecode {
	// ensure halt
	if len(c.instructions) == 0 || Opcode(c.instructions[len(c.instructions)-1]) != OpHalt {
		c.emit(OpHalt)
	}
	return &Bytecode{
		Instructions: append([]byte{}, c.instructions...),
		Constants:    append([]interface{}{}, c.constants...),
		Names:        append([]string{}, c.names...),
	}
}

func (c *Compiler) Compile(node ast.Node) error {
	switch n := node.(type) {
	case *ast.Program:
		for _, s := range n.Statements {
			if err := c.Compile(s); err != nil {
				return err
			}
		}
	case *ast.ExpressionStatement:
		if err := c.Compile(n.Expression); err != nil {
			return err
		}
		c.emit(OpPop)
	case *ast.IntegerLiteral:
		c.emit(OpConstant, c.addConst(n.Value))
	case *ast.FloatLiteral:
		c.emit(OpConstant, c.addConst(n.Value))
	case *ast.StringLiteral:
		c.emit(OpConstant, c.addConst(n.Value))
	case *ast.Boolean:
		if n.Value {
			c.emit(OpTrue)
		} else {
			c.emit(OpFalse)
		}
	case *ast.NullLiteral:
		c.emit(OpNull)
	case *ast.PrefixExpression:
		if err := c.Compile(n.Right); err != nil {
			return err
		}
		switch n.Operator {
		case "-":
			c.emit(OpMinus)
		case "!":
			c.emit(OpBang)
		default:
			return fmt.Errorf("bytecode: unsupported prefix %s", n.Operator)
		}
	case *ast.InfixExpression:
		// special: short-circuit could be jumps; keep simple for now
		if err := c.Compile(n.Left); err != nil {
			return err
		}
		if err := c.Compile(n.Right); err != nil {
			return err
		}
		switch n.Operator {
		case "+":
			c.emit(OpAdd)
		case "-":
			c.emit(OpSub)
		case "*":
			c.emit(OpMul)
		case "/":
			c.emit(OpDiv)
		case "%":
			c.emit(OpMod)
		case "==":
			c.emit(OpEqual)
		case "!=":
			c.emit(OpNotEqual)
		case ">":
			c.emit(OpGreater)
		case "<":
			c.emit(OpLess)
		case ">=":
			c.emit(OpGreaterEq)
		case "<=":
			c.emit(OpLessEq)
		default:
			return fmt.Errorf("bytecode: unsupported infix %s", n.Operator)
		}
	case *ast.LetStatement:
		if err := c.Compile(n.Value); err != nil {
			return err
		}
		c.emit(OpSetGlobal, c.globalIndex(n.Name.Value))
	case *ast.ConstStatement:
		if err := c.Compile(n.Value); err != nil {
			return err
		}
		c.emit(OpSetGlobal, c.globalIndex(n.Name.Value))
	case *ast.Identifier:
		// builtins handled partially: print is CallExpression
		c.emit(OpGetGlobal, c.globalIndex(n.Value))
	case *ast.PrintStatement:
		if err := c.Compile(n.Value); err != nil {
			return err
		}
		c.emit(OpPrint)
	case *ast.AssignExpression:
		if err := c.Compile(n.Value); err != nil {
			return err
		}
		c.emit(OpSetGlobal, c.globalIndex(n.Name.Value))
		// leave value on stack? assignment as expr — push back
		c.emit(OpGetGlobal, c.globalIndex(n.Name.Value))
	case *ast.IfExpression:
		// condition
		if err := c.Compile(n.Condition); err != nil {
			return err
		}
		jnt := c.emit(OpJumpNotTruthy, 0) // placeholder
		if err := c.Compile(n.Consequence); err != nil {
			return err
		}
		jmp := c.emit(OpJump, 0)
		// patch jump-not-truthy to here
		c.patch(jnt, len(c.instructions))
		if n.Alternative != nil {
			if err := c.Compile(n.Alternative); err != nil {
				return err
			}
		} else {
			c.emit(OpNull)
		}
		c.patch(jmp, len(c.instructions))
	case *ast.BlockStatement:
		var last ast.Statement
		for _, s := range n.Statements {
			last = s
			if err := c.Compile(s); err != nil {
				return err
			}
		}
		// if block ends with expression statement we already OpPop'd — for if value, keep last expr
		_ = last
	case *ast.CallExpression:
		// only support print(x) style via identifier print
		if id, ok := n.Function.(*ast.Identifier); ok && id.Value == "print" {
			for _, a := range n.Arguments {
				if err := c.Compile(a); err != nil {
					return err
				}
				c.emit(OpPrint)
			}
			c.emit(OpNull)
			return nil
		}
		return fmt.Errorf("bytecode: only print() calls supported in this stage")
	default:
		return fmt.Errorf("bytecode: unsupported node %T", node)
	}
	return nil
}

func (c *Compiler) addConst(v interface{}) int {
	c.constants = append(c.constants, v)
	return len(c.constants) - 1
}

func (c *Compiler) globalIndex(name string) int {
	if i, ok := c.nameIndex[name]; ok {
		return i
	}
	i := len(c.names)
	c.names = append(c.names, name)
	c.nameIndex[name] = i
	return i
}

func (c *Compiler) emit(op Opcode, operands ...int) int {
	pos := len(c.instructions)
	c.instructions = append(c.instructions, byte(op))
	for _, o := range operands {
		c.instructions = append(c.instructions, byte(o>>8), byte(o))
	}
	return pos
}

func (c *Compiler) patch(opPos int, target int) {
	// opPos points at opcode; operands are next 2 bytes
	c.instructions[opPos+1] = byte(target >> 8)
	c.instructions[opPos+2] = byte(target)
}

// Disassemble returns human-readable bytecode.
func Disassemble(bc *Bytecode) string {
	var out string
	ip := 0
	ins := bc.Instructions
	for ip < len(ins) {
		op := Opcode(ins[ip])
		line := fmt.Sprintf("%04d %s", ip, op.String())
		ip++
		switch op {
		case OpConstant:
			idx := ReadUint16(ins, ip)
			ip += 2
			line += fmt.Sprintf(" %d (%v)", idx, bc.Constants[idx])
		case OpJump, OpJumpNotTruthy:
			t := ReadUint16(ins, ip)
			ip += 2
			line += fmt.Sprintf(" -> %d", t)
		case OpSetGlobal, OpGetGlobal:
			idx := ReadUint16(ins, ip)
			ip += 2
			name := "?"
			if idx < len(bc.Names) {
				name = bc.Names[idx]
			}
			line += fmt.Sprintf(" %d (%s)", idx, name)
		}
		out += line + "\n"
	}
	return out
}
