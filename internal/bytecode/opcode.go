// Package bytecode implements a stack-based VM and compiler for NvS.
//
// Honest scope (this stage): integer/float/string literals, arithmetic
// (+, -, *, /, % with truncating int division and explicit division/modulo
// by zero errors), string concatenation and lexicographic comparison,
// comparisons, unary -/!, booleans, null, let/const globals, assignment,
// if/else, while and C-style for loops with break/continue, print(...),
// len() over strings and arrays, and array literals.
//
// NOT supported — the compiler fails loudly with "bytecode: unsupported ..."
// for anything else: function definitions and calls (other than print/len),
// for-in loops, while/for-else, labeled break/continue, tuples, hashes,
// records, classes, string interpolation inside the VM, and all other
// builtins. The tree-walking interpreter remains the reference
// implementation; the VM is experimental.
package bytecode

import "fmt"

type Opcode byte

const (
	OpConstant Opcode = iota // operand: const index
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpPow
	OpEqual
	OpNotEqual
	OpGreater
	OpLess
	OpGreaterEq
	OpLessEq
	OpMinus // unary
	OpBang
	OpTrue
	OpFalse
	OpNull
	OpPop
	OpJump // operand: absolute IP
	OpJumpNotTruthy
	OpSetGlobal // operand: global index
	OpGetGlobal
	OpPrint // operand: arg count; pop N, print space-joined on one line
	OpLen   // pop value, push its length (string bytes, array elements)
	OpArray // operand: element count; pop N values, push []interface{}
	OpHalt
)

var opNames = map[Opcode]string{
	OpConstant: "Constant", OpAdd: "Add", OpSub: "Sub", OpMul: "Mul", OpDiv: "Div",
	OpMod: "Mod", OpPow: "Pow", OpEqual: "Equal", OpNotEqual: "NotEqual", OpGreater: "Greater",
	OpLess: "Less", OpGreaterEq: "GreaterEq", OpLessEq: "LessEq", OpMinus: "Minus",
	OpBang: "Bang", OpTrue: "True", OpFalse: "False", OpNull: "Null", OpPop: "Pop",
	OpJump: "Jump", OpJumpNotTruthy: "JumpNotTruthy", OpSetGlobal: "SetGlobal",
	OpGetGlobal: "GetGlobal", OpPrint: "Print", OpLen: "Len", OpArray: "Array",
	OpHalt: "Halt",
}

func (op Opcode) String() string {
	if s, ok := opNames[op]; ok {
		return s
	}
	return fmt.Sprintf("Op(%d)", op)
}

// Operand widths (bytes after opcode)
func OperandWidth(op Opcode) int {
	switch op {
	case OpConstant, OpJump, OpJumpNotTruthy, OpSetGlobal, OpGetGlobal, OpArray, OpPrint:
		return 2 // uint16
	default:
		return 0
	}
}

func ReadUint16(ins []byte, ip int) int {
	return int(ins[ip])<<8 | int(ins[ip+1])
}
