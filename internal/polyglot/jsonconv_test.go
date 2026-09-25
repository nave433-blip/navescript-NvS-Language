package polyglot

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/object"
)

// nvsToJSONBytes is the full NvS -> JSON-text path the bridges use.
func nvsToJSONBytes(t *testing.T, obj object.Object) string {
	t.Helper()
	v, err := NvSValueToJSON(obj)
	if err != nil {
		t.Fatalf("NvSValueToJSON: %v", err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(data)
}

// jsonBytesToNvS is the full JSON-text -> NvS path (UseNumber, like the bridges).
func jsonBytesToNvS(t *testing.T, data string) object.Object {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(data)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	obj, err := JSONValueToNvS(v)
	if err != nil {
		t.Fatalf("JSONValueToNvS(%q): %v", data, err)
	}
	return obj
}

func TestNvSValueToJSONPrimitives(t *testing.T) {
	cases := []struct {
		obj  object.Object
		want string
	}{
		{&object.Integer{Value: 42}, `42`},
		{&object.Integer{Value: -7}, `-7`},
		{&object.Float{Value: 1.5}, `1.5`},
		{&object.String{Value: "hi"}, `"hi"`},
		{&object.String{Value: "a\"b\\c"}, `"a\"b\\c"`},
		{&object.Boolean{Value: true}, `true`},
		{&object.Boolean{Value: false}, `false`},
		{&object.Null{}, `null`},
	}
	for _, c := range cases {
		if got := nvsToJSONBytes(t, c.obj); got != c.want {
			t.Errorf("NvS->JSON(%v) = %s, want %s", c.obj, got, c.want)
		}
	}
}

func TestNvSValueToJSONNested(t *testing.T) {
	obj := &object.Array{Elements: []object.Object{
		&object.Integer{Value: 1},
		&object.String{Value: "x"},
		&object.Boolean{Value: true},
		&object.Null{},
		&object.Array{Elements: []object.Object{&object.Float{Value: 2.5}}},
	}}
	got := nvsToJSONBytes(t, obj)
	want := `[1,"x",true,null,[2.5]]`
	if got != want {
		t.Errorf("NvS->JSON(nested) = %s, want %s", got, want)
	}
}

func TestNvSValueToJSONHashKeys(t *testing.T) {
	mkHash := func(pairs ...object.Object) *object.Hash {
		h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
		for i := 0; i < len(pairs); i += 2 {
			k, ok := pairs[i].(object.Hashable)
			if !ok {
				t.Fatalf("key %v not hashable", pairs[i])
			}
			h.Pairs[k.HashKey()] = object.HashPair{Key: pairs[i], Value: pairs[i+1]}
		}
		return h
	}
	h := mkHash(
		&object.String{Value: "a"}, &object.Integer{Value: 1},
		&object.Integer{Value: 2}, &object.String{Value: "two"},
		&object.Boolean{Value: true}, &object.Null{},
	)
	got := nvsToJSONBytes(t, h)
	for _, want := range []string{`"a":1`, `"2":"two"`, `"true":null`} {
		if !strings.Contains(got, want) {
			t.Errorf("NvS->JSON(hash) = %s, missing %s", got, want)
		}
	}
	// Collision: int key 1 and string key "1" both render as "1".
	coll := mkHash(
		&object.Integer{Value: 1}, &object.String{Value: "int"},
		&object.String{Value: "1"}, &object.String{Value: "str"},
	)
	if _, err := NvSValueToJSON(coll); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Errorf("colliding keys should report collision, got %v", err)
	}
	// Non-string/int/bool keys are rejected, naming the type. (Only
	// string/int/bool are Hashable in NvS, so this path is defensive;
	// the key is smuggled in directly.)
	intKey := (&object.Integer{Value: 9}).HashKey()
	badKey := &object.Hash{Pairs: map[object.HashKey]object.HashPair{
		intKey: {Key: &object.Array{Elements: nil}, Value: &object.Integer{Value: 1}},
	}}
	if _, err := NvSValueToJSON(badKey); err == nil || !strings.Contains(err.Error(), "ARRAY") {
		t.Errorf("array key should name ARRAY, got %v", err)
	}
}

func TestNvSValueToJSONUnsupportedNamesType(t *testing.T) {
	// Unsupported NvS values yield an error naming the NvS type —
	// never a silent mis-conversion.
	for _, obj := range []object.Object{
		&object.Function{},
		&object.Builtin{},
	} {
		_, err := NvSValueToJSON(obj)
		if err == nil || !strings.Contains(err.Error(), string(obj.Type())) {
			t.Errorf("NvSValueToJSON(%s) should name the type, got %v", obj.Type(), err)
		}
	}
	bad := &object.Array{Elements: []object.Object{&object.Function{}}}
	if _, err := NvSValueToJSON(bad); err == nil || !strings.Contains(err.Error(), "FUNCTION") {
		t.Errorf("NvSValueToJSON([fn]) should name FUNCTION, got %v", err)
	}
}

func TestJSONValueToNvSRoundTrip(t *testing.T) {
	for _, src := range []string{
		`42`, `-7`, `1.5`, `"hi"`, `true`, `false`, `null`,
		`[1, "a", true, null, [2], {"k": 3}]`,
		`{"a": 1, "b": [true]}`,
		`9007199254740993`, // big int stays an NvS integer, not a float
		`""`, `[]`, `{}`,
	} {
		obj := jsonBytesToNvS(t, src)
		back := nvsToJSONBytes(t, obj)
		obj2 := jsonBytesToNvS(t, back)
		back2 := nvsToJSONBytes(t, obj2)
		if back != back2 {
			t.Errorf("round trip unstable for %s: %s vs %s", src, back, back2)
		}
	}
	// Integer syntax stays integer.
	if _, ok := jsonBytesToNvS(t, `9007199254740993`).(*object.Integer); !ok {
		t.Errorf("big int did not decode as *object.Integer")
	}
	// 1.0 decodes as float.
	if _, ok := jsonBytesToNvS(t, `1.0`).(*object.Float); !ok {
		t.Errorf("1.0 did not decode as *object.Float")
	}
	// Nested arrays/hashes keep their NvS types.
	obj := jsonBytesToNvS(t, `[1, {"a": [true]}]`)
	arr, ok := obj.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("array decode: %T", obj)
	}
	if _, ok := arr.Elements[0].(*object.Integer); !ok {
		t.Errorf("elements[0] = %T", arr.Elements[0])
	}
	h, ok := arr.Elements[1].(*object.Hash)
	if !ok || len(h.Pairs) != 1 {
		t.Fatalf("hash decode: %T", arr.Elements[1])
	}
}

func TestJSONValueToNvSErrors(t *testing.T) {
	for _, src := range []string{``, `   `, `[1,`, `{"a":}`, `tru`, `"unterminated`} {
		dec := json.NewDecoder(bytes.NewReader([]byte(src)))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil {
			// Decodable but unexpected Go type (shouldn't happen for these).
			if _, err := JSONValueToNvS(v); err == nil {
				t.Errorf("JSONValueToNvS(%q) should fail", src)
			}
		}
	}
	if _, err := JSONValueToNvS(complex(1, 2)); err == nil {
		t.Error("complex128 should fail")
	}
}

func TestDecodeJSONArgs(t *testing.T) {
	args, err := DecodeJSONArgs([]byte(`[1, "a", null, 1.5, true]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 5 {
		t.Fatalf("got %d args", len(args))
	}
	// UseNumber: integer syntax arrives as json.Number.
	if _, ok := args[0].(json.Number); !ok {
		t.Errorf("args[0] = %T, want json.Number", args[0])
	}
	// And it converts to an NvS integer, not a float.
	obj, err := JSONValueToNvS(args[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj.(*object.Integer); !ok {
		t.Errorf("args[0] converted to %T, want *object.Integer", obj)
	}
	for _, bad := range []string{
		`[1,]`, `1`, `"s"`, `{"a":1}`, ``, `[1] garbage`, `[1] [2]`,
	} {
		if _, err := DecodeJSONArgs([]byte(bad)); err == nil {
			t.Errorf("DecodeJSONArgs(%q) should fail", bad)
		}
	}
}

func TestEnvelopeJSON(t *testing.T) {
	ok := EnvelopeJSON(&object.Integer{Value: 1}, nil)
	if ok != `{"ok":true,"result":1}` {
		t.Errorf("ok envelope = %s", ok)
	}
	// A nil object with a nil error is NvS null.
	nullEnv := EnvelopeJSON(nil, nil)
	if nullEnv != `{"ok":true,"result":null}` {
		t.Errorf("null envelope = %s", nullEnv)
	}
	errEnv := EnvelopeJSON(nil, errors.New(`boom "x"`))
	// Note: encoding/json sorts map keys, so "error" comes first.
	if errEnv != `{"error":"boom \"x\"","ok":false}` {
		t.Errorf("error envelope = %s", errEnv)
	}
	// An unconvertible result becomes ok:false naming the type.
	fnEnv := EnvelopeJSON(&object.Function{}, nil)
	if !strings.Contains(fnEnv, `"ok":false`) || !strings.Contains(fnEnv, "FUNCTION") {
		t.Errorf("function envelope = %s", fnEnv)
	}
	// A nested unconvertible value is also honest.
	nested := &object.Array{Elements: []object.Object{&object.Builtin{}}}
	nestedEnv := EnvelopeJSON(nested, nil)
	if !strings.Contains(nestedEnv, `"ok":false`) || !strings.Contains(nestedEnv, "BUILTIN") {
		t.Errorf("nested envelope = %s", nestedEnv)
	}
}
