package polyglot

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/navescript/nvs/internal/object"
)

// Wave 10: JSON value conversion shared by the C ABI bridge (cbridge) and
// the JSON stdio bridge (`nvs bridge`). Both directions are total over the
// documented convertible set and fail loudly everywhere else.

// NvSValueToJSON converts an NvS object into a JSON-compatible Go value
// (suitable for encoding/json). Convertible NvS types:
//
//	INTEGER, FLOAT, STRING, BOOLEAN, NULL, ARRAY, HASH
//
// Hash keys must be STRING, INTEGER, or BOOLEAN — JSON object keys are
// strings, so integer/boolean keys are rendered in their natural notation
// ("1", "true"). A key collision after rendering is an error, not a silent
// overwrite. Any other NvS type yields an error naming the type: values are
// never silently mis-converted.
func NvSValueToJSON(obj object.Object) (any, error) {
	switch o := obj.(type) {
	case *object.Integer:
		return o.Value, nil
	case *object.Float:
		return o.Value, nil
	case *object.String:
		return o.Value, nil
	case *object.Boolean:
		return o.Value, nil
	case *object.Null:
		return nil, nil
	case *object.Array:
		out := make([]any, len(o.Elements))
		for i, el := range o.Elements {
			v, err := NvSValueToJSON(el)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case *object.Hash:
		out := make(map[string]any, len(o.Pairs))
		for _, pair := range o.Pairs {
			var key string
			switch k := pair.Key.(type) {
			case *object.String:
				key = k.Value
			case *object.Integer:
				key = fmt.Sprintf("%d", k.Value)
			case *object.Boolean:
				key = fmt.Sprintf("%t", k.Value)
			default:
				return nil, fmt.Errorf("cannot convert NvS hash key of type %s to JSON (keys must be string, int, or bool)", pair.Key.Type())
			}
			if _, exists := out[key]; exists {
				return nil, fmt.Errorf("cannot convert NvS hash to JSON: key %q collides after key rendering", key)
			}
			v, err := NvSValueToJSON(pair.Value)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cannot convert NvS %s to JSON (supported: int, float, string, bool, null, array, hash)", obj.Type())
	}
}

// JSONValueToNvS converts a JSON-decoded Go value into an NvS object.
// It expects values produced by encoding/json with UseNumber() (so that
// integer-syntax numbers arrive as json.Number and become NvS INTEGERs);
// plain float64 values (decoder without UseNumber) become NvS FLOATs.
// JSON object keys always become NvS strings — a one-way rendering, since
// JSON has no non-string keys.
func JSONValueToNvS(v any) (object.Object, error) {
	switch t := v.(type) {
	case nil:
		return &object.Null{}, nil
	case bool:
		return &object.Boolean{Value: t}, nil
	case string:
		return &object.String{Value: t}, nil
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return &object.Integer{Value: i}, nil
		}
		f, err := t.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid JSON number %q", t.String())
		}
		return &object.Float{Value: f}, nil
	case float64:
		return &object.Float{Value: t}, nil
	case []any:
		els := make([]object.Object, len(t))
		for i, e := range t {
			o, err := JSONValueToNvS(e)
			if err != nil {
				return nil, err
			}
			els[i] = o
		}
		return &object.Array{Elements: els}, nil
	case map[string]any:
		pairs := make(map[object.HashKey]object.HashPair, len(t))
		for k, e := range t {
			key := &object.String{Value: k}
			val, err := JSONValueToNvS(e)
			if err != nil {
				return nil, err
			}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: val}
		}
		return &object.Hash{Pairs: pairs}, nil
	default:
		return nil, fmt.Errorf("cannot convert Go value of type %T to NvS", v)
	}
}

// DecodeJSONArgs decodes a JSON array (e.g. the "args" of an nvs_call /
// bridge {"call":...} request) using UseNumber so integers survive as
// json.Number. Anything that is not a JSON array is an error, and so is
// trailing data after the array — `[1] garbage` is rejected, not
// silently truncated.
func DecodeJSONArgs(data []byte) ([]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON args: %s", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("invalid JSON args: trailing data after JSON array")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid JSON args: want a JSON array")
	}
	return arr, nil
}

// EnvelopeJSON builds the response envelope shared by the C ABI bridge and
// the JSON stdio bridge:
//
//	{"ok":true,"result":<json>}   on success
//	{"ok":false,"error":"..."}    on failure (eval error, call error, or a
//	                              result value outside the convertible set)
//
// A nil object with a nil error is treated as NvS null.
func EnvelopeJSON(obj object.Object, callErr error) string {
	var m map[string]any
	switch {
	case callErr != nil:
		m = map[string]any{"ok": false, "error": callErr.Error()}
	case obj == nil:
		m = map[string]any{"ok": true, "result": nil}
	default:
		v, err := NvSValueToJSON(obj)
		if err != nil {
			m = map[string]any{"ok": false, "error": err.Error()}
		} else {
			m = map[string]any{"ok": true, "result": v}
		}
	}
	data, err := json.Marshal(m)
	if err != nil {
		return `{"ok":false,"error":"failed to encode response envelope"}`
	}
	return string(data)
}
