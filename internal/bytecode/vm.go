package bytecode

import (
	"fmt"
	"strings"
)

type VM struct {
	ins       []byte
	constants []interface{}
	names     []string
	stack     []interface{}
	sp        int
	globals   []interface{}
	Output    strings.Builder // captured prints
}

func NewVM(bc *Bytecode) *VM {
	g := make([]interface{}, len(bc.Names)+64)
	return &VM{
		ins:       bc.Instructions,
		constants: bc.Constants,
		names:     bc.Names,
		stack:     make([]interface{}, 1024),
		globals:   g,
	}
}

func (vm *VM) push(v interface{}) {
	vm.stack[vm.sp] = v
	vm.sp++
}

func (vm *VM) pop() interface{} {
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) Run() (interface{}, error) {
	ip := 0
	for ip < len(vm.ins) {
		op := Opcode(vm.ins[ip])
		ip++
		switch op {
		case OpConstant:
			idx := ReadUint16(vm.ins, ip)
			ip += 2
			vm.push(vm.constants[idx])
		case OpAdd:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return a + b }))
		case OpSub:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return a - b }))
		case OpMul:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return a * b }))
		case OpDiv:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return a / b }))
		case OpMod:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return float64(int64(a) % int64(b)) }))
		case OpEqual:
			r, l := vm.pop(), vm.pop()
			vm.push(eq(l, r))
		case OpNotEqual:
			r, l := vm.pop(), vm.pop()
			vm.push(!eq(l, r))
		case OpGreater:
			r, l := vm.pop(), vm.pop()
			vm.push(toF(l) > toF(r))
		case OpLess:
			r, l := vm.pop(), vm.pop()
			vm.push(toF(l) < toF(r))
		case OpGreaterEq:
			r, l := vm.pop(), vm.pop()
			vm.push(toF(l) >= toF(r))
		case OpLessEq:
			r, l := vm.pop(), vm.pop()
			vm.push(toF(l) <= toF(r))
		case OpMinus:
			v := vm.pop()
			vm.push(-toF(v))
		case OpBang:
			v := vm.pop()
			vm.push(!truthy(v))
		case OpTrue:
			vm.push(true)
		case OpFalse:
			vm.push(false)
		case OpNull:
			vm.push(nil)
		case OpPop:
			if vm.sp > 0 {
				vm.pop()
			}
		case OpJump:
			ip = ReadUint16(vm.ins, ip)
		case OpJumpNotTruthy:
			target := ReadUint16(vm.ins, ip)
			ip += 2
			cond := vm.pop()
			if !truthy(cond) {
				ip = target
			}
		case OpSetGlobal:
			idx := ReadUint16(vm.ins, ip)
			ip += 2
			vm.globals[idx] = vm.pop()
		case OpGetGlobal:
			idx := ReadUint16(vm.ins, ip)
			ip += 2
			vm.push(vm.globals[idx])
		case OpPrint:
			v := vm.pop()
			s := fmt.Sprint(v)
			vm.Output.WriteString(s)
			vm.Output.WriteByte('\n')
			fmt.Println(s)
		case OpHalt:
			if vm.sp > 0 {
				return vm.pop(), nil
			}
			return nil, nil
		default:
			return nil, fmt.Errorf("unknown opcode %s at %d", op, ip-1)
		}
	}
	if vm.sp > 0 {
		return vm.pop(), nil
	}
	return nil, nil
}

func toF(v interface{}) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func numBin(l, r interface{}, f func(a, b float64) float64) interface{} {
	// prefer int if both int-like
	_, li := l.(int64)
	_, ri := r.(int64)
	if li && ri {
		a, b := l.(int64), r.(int64)
		// detect op via float result pattern — use f then convert
		res := f(float64(a), float64(b))
		if res == float64(int64(res)) {
			return int64(res)
		}
		return res
	}
	return f(toF(l), toF(r))
}

func eq(l, r interface{}) bool {
	switch a := l.(type) {
	case int64:
		switch b := r.(type) {
		case int64:
			return a == b
		case float64:
			return float64(a) == b
		}
	case float64:
		return a == toF(r)
	case string:
		b, ok := r.(string)
		return ok && a == b
	case bool:
		b, ok := r.(bool)
		return ok && a == b
	case nil:
		return r == nil
	}
	return fmt.Sprint(l) == fmt.Sprint(r)
}

func truthy(v interface{}) bool {
	switch n := v.(type) {
	case nil:
		return false
	case bool:
		return n
	case int64:
		return n != 0
	case float64:
		return n != 0
	case string:
		return n != ""
	default:
		return true
	}
}
