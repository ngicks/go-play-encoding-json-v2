package play

import (
	"bytes"
	"encoding/json/jsontext"
	"io"
	"strings"
	"testing"
)

func TestDecoder(t *testing.T) {
	const input = `{
    "foo": null,
    "baz": [
        "qux",
        123,
        "quux",
        [
            {
                "corge": null
            }
        ]
    ]
}
`
	dec := jsontext.NewDecoder(strings.NewReader(input))

	expected := []any{
		jsontext.BeginObject,
		jsontext.String("foo"),
		jsontext.Null,
		jsontext.String("baz"),
		"peek",
		jsontext.BeginArray,
		jsontext.String("qux"),
		jsontext.Int(123),
		"peek",
		jsontext.String("quux"),
		jsontext.Value(`[{"corge":null}]`),
		jsontext.EndArray,
		jsontext.EndObject,
	}

	for _, tokenOrValue := range expected {
		idxKind, valueLen := dec.StackIndex(dec.StackDepth())
		t.Logf("depth = %d, index kind = %s, len at index = %d, stack pointer = %q", dec.StackDepth(), idxKind, valueLen, dec.StackPointer())
		/*
		   decoder_test.go:46: depth = 0, index kind = <invalid jsontext.Kind: '\x00'>, len at index = 0, stack pointer = ""
		   decoder_test.go:46: depth = 1, index kind = {, len at index = 0, stack pointer = ""
		   decoder_test.go:46: depth = 1, index kind = {, len at index = 1, stack pointer = "/foo"
		   decoder_test.go:46: depth = 1, index kind = {, len at index = 2, stack pointer = "/foo"
		   decoder_test.go:46: depth = 1, index kind = {, len at index = 3, stack pointer = "/baz"
		   decoder_test.go:46: depth = 1, index kind = {, len at index = 3, stack pointer = "/baz"
		   decoder_test.go:46: depth = 2, index kind = [, len at index = 0, stack pointer = "/baz"
		   decoder_test.go:46: depth = 2, index kind = [, len at index = 1, stack pointer = "/baz/0"
		   decoder_test.go:46: depth = 2, index kind = [, len at index = 2, stack pointer = "/baz/1"
		   decoder_test.go:46: depth = 2, index kind = [, len at index = 2, stack pointer = "/baz/1"
		   decoder_test.go:46: depth = 2, index kind = [, len at index = 3, stack pointer = "/baz/2"
		   decoder_test.go:46: depth = 2, index kind = [, len at index = 4, stack pointer = "/baz/3"
		   decoder_test.go:46: depth = 1, index kind = {, len at index = 4, stack pointer = "/baz"
		*/
		switch x := tokenOrValue.(type) {
		case string:
			t.Logf("peek = %s", dec.PeekKind())
		/*
		   decoder_test.go:62: peek = [
		   decoder_test.go:62: peek = string
		*/
		case jsontext.Token:
			tok, err := dec.ReadToken()
			if err != nil && err != io.EOF {
				panic(err)
			}
			if tok.Kind() != x.Kind() {
				t.Errorf("not equal: expected(%v) != actual(%v)", x, tok)
			}
			switch tok.Kind() {
			case 'n': // null
			case 'f': // false
			case 't': // true
			case '"', '0': // string literal, number literal
				if tok.String() != x.String() {
					t.Errorf("not equal: expected(%s) != actual(%s)", x, tok)
				}
			case '{': // end object
			case '}': // end object
			case '[': // begin array
			case ']': // end array
			}
		case jsontext.Value:
			val, err := dec.ReadValue()
			if err != nil && err != io.EOF {
				panic(err)
			}
			if !bytes.Equal(mustCompact(val), mustCompact(x)) {
				t.Errorf("not equal: expected(%q) != actual(%q)", string(x), string(val))
			}
		}
	}
}

func mustCompact(v jsontext.Value) jsontext.Value {
	err := v.Compact()
	if err != nil {
		panic(err)
	}
	return v
}
