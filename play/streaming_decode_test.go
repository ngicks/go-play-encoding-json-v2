package play

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"reflect"
	"testing"
)

func TestStreamingDecode(t *testing.T) {
	const input = `{
    "foo": null,
    "bar": {
            "baz": [
                {"foo":"foo1"},
                {"foo":"foo2"},
                {"foo":"foo3"}
            ]
        }
}
`

	dec := jsontext.NewDecoder(bytes.NewReader([]byte(input)))
	for dec.StackPointer() != jsontext.Pointer("/bar/baz") {
		_, err := dec.ReadToken()
		if err != nil {
			if err != io.EOF {
				panic(err)
			}
			break
		}
	}

	if dec.PeekKind() != '[' {
		panic("not array")
	}
	_, err := dec.ReadToken()
	if err != nil {
		panic(err)
	}
	type sample struct {
		Foo string `json:"foo"`
	}
	var decoded []sample
	for dec.PeekKind() != ']' {
		var s sample
		err := json.UnmarshalDecode(dec, &s)
		if err != nil {
			panic(err)
		}
		decoded = append(decoded, s)
	}
	expected := []sample{{"foo1"}, {"foo2"}, {"foo3"}}
	if !reflect.DeepEqual(expected, decoded) {
		t.Errorf("not equal:\nexpected(%#v)\n!=\nactual(%#v)", expected, decoded)
	} else {
		t.Logf("decoded = %#v", decoded)
		// streaming_decode_test.go:59: decoded = []play.sample{play.sample{Foo:"foo1"}, play.sample{Foo:"foo2"}, play.sample{Foo:"foo3"}}
	}
}
