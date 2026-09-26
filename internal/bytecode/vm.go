package bytecode

import (
	"fmt"
	"math"
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
			if ls, ok := l.(string); ok {
				if rs, ok := r.(string); ok {
					vm.push(ls + rs)
					break
				}
				return nil, fmt.Errorf("type mismatch: string + %s", typeName(r))
			}
			if _, ok := r.(string); ok {
				return nil, fmt.Errorf("type mismatch: %s + string", typeName(l))
			}
			vm.push(numBin(l, r, func(a, b float64) float64 { return a + b }))
		case OpSub:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return a - b }))
		case OpMul:
			r, l := vm.pop(), vm.pop()
			vm.push(numBin(l, r, func(a, b float64) float64 { return a * b }))
		case OpDiv:
			r, l := vm.pop(), vm.pop()
			v, err := numDiv(l, r)
			if err != nil {
				return nil, err
			}
			vm.push(v)
		case OpMod:
			r, l := vm.pop(), vm.pop()
			v, err := numMod(l, r)
			if err != nil {
				return nil, err
			}
			vm.push(v)
		case OpPow:
			r, l := vm.pop(), vm.pop()
			vm.push(numPow(l, r))
		case OpEqual:
			r, l := vm.pop(), vm.pop()
			vm.push(eq(l, r))
		case OpNotEqual:
			r, l := vm.pop(), vm.pop()
			vm.push(!eq(l, r))
		case OpGreater:
			r, l := vm.pop(), vm.pop()
			v, err := cmp(l, r, func(c int) bool { return c > 0 })
			if err != nil {
				return nil, err
			}
			vm.push(v)
		case OpLess:
			r, l := vm.pop(), vm.pop()
			v, err := cmp(l, r, func(c int) bool { return c < 0 })
			if err != nil {
				return nil, err
			}
			vm.push(v)
		case OpGreaterEq:
			r, l := vm.pop(), vm.pop()
			v, err := cmp(l, r, func(c int) bool { return c >= 0 })
			if err != nil {
				return nil, err
			}
			vm.push(v)
		case OpLessEq:
			r, l := vm.pop(), vm.pop()
			v, err := cmp(l, r, func(c int) bool { return c <= 0 })
			if err != nil {
				return nil, err
			}
			vm.push(v)
		case OpMinus:
			v := vm.pop()
			if i, ok := v.(int64); ok {
				vm.push(-i)
			} else {
				vm.push(-toF(v))
			}
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
			n := ReadUint16(vm.ins, ip)
			ip += 2
			parts := make([]string, n)
			for i := n - 1; i >= 0; i-- {
				parts[i] = fmt.Sprint(vm.pop())
			}
			s := strings.Join(parts, " ")
			vm.Output.WriteString(s)
			vm.Output.WriteByte('\n')
			fmt.Println(s)
		case OpLen:
			v := vm.pop()
			switch t := v.(type) {
			case string:
				vm.push(int64(len(t))) // byte count, like the tree-walker
			case []interface{}:
				vm.push(int64(len(t)))
			default:
				return nil, fmt.Errorf("argument to `len` not supported, got %s", typeName(v))
			}
		case OpArray:
			n := ReadUint16(vm.ins, ip)
			ip += 2
			elems := make([]interface{}, n)
			for i := n - 1; i >= 0; i-- {
				elems[i] = vm.pop()
			}
			vm.push(elems)
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

func typeName(v interface{}) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case int64, int:
		return "int"
	case float64:
		return "float"
	case string:
		return "string"
	case []interface{}:
		return "array"
	default:
		return "unknown"
	}
}

// numDiv matches the tree-walker: truncating division for two ints,
// float division otherwise, and an explicit error on division by zero.
func numDiv(l, r interface{}) (interface{}, error) {
	if li, ok := l.(int64); ok {
		if ri, ok := r.(int64); ok {
			if ri == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return li / ri, nil
		}
	}
	rf := toF(r)
	if rf == 0 {
		return nil, fmt.Errorf("division by zero")
	}
	return toF(l) / rf, nil
}

// numMod matches the tree-walker: Go % semantics for two ints with an
// explicit error on modulo by zero (the tree-walker panics there — the
// VM is honest instead).
func numMod(l, r interface{}) (interface{}, error) {
	if li, ok := l.(int64); ok {
		if ri, ok := r.(int64); ok {
			if ri == 0 {
				return nil, fmt.Errorf("modulo by zero")
			}
			return li % ri, nil
		}
	}
	return nil, fmt.Errorf("type mismatch: %s %% %s", typeName(l), typeName(r))
}

// cmp compares two values: lexicographic for two strings (like the
// tree-walker), numeric otherwise. pred maps strings.Compare's result.
func cmp(l, r interface{}, pred func(int) bool) (bool, error) {
	if ls, ok := l.(string); ok {
		if rs, ok := r.(string); ok {
			return pred(strings.Compare(ls, rs)), nil
		}
		return false, fmt.Errorf("type mismatch: string compared with %s", typeName(r))
	}
	if _, ok := r.(string); ok {
		return false, fmt.Errorf("type mismatch: %s compared with string", typeName(l))
	}
	lf, rf := toF(l), toF(r)
	switch {
	case lf < rf:
		return pred(-1), nil
	case lf > rf:
		return pred(1), nil
	default:
		return pred(0), nil
	}
}

func numPow(l, r interface{}) interface{} {
	// Mirror the tree-walker: int**int is integer power (negative exponent -> 0),
	// anything else goes through math.Pow.
	if li, ok := l.(int64); ok {
		if ri, ok := r.(int64); ok {
			if ri < 0 {
				return int64(0)
			}
			var p int64 = 1
			for i := int64(0); i < ri; i++ {
				p *= li
			}
			return p
		}
	}
	return math.Pow(toF(l), toF(r))
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
