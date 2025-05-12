package play

import (
	"bytes"
	"encoding/json/jsontext"
	"testing"
)

func TestEncoder(t *testing.T) {
	buf := new(bytes.Buffer)
	enc := jsontext.NewEncoder(buf, jsontext.WithIndent("    "))

	var err error
	bufErr := func(e error) {
		if err != nil {
			return
		}
		err = e
	}

	assertDepth := func(enc *jsontext.Encoder, depth int) {
		if enc.StackDepth() != depth {
			t.Errorf("wrong depth: expected = %d, actual = %d", depth, enc.StackDepth())
		}
	}

	assertDepth(enc, 0)
	bufErr(enc.WriteToken(jsontext.BeginObject))
	assertDepth(enc, 1)

	bufErr(enc.WriteToken(jsontext.String("foo")))
	bufErr(enc.WriteToken(jsontext.Null))
	bufErr(enc.WriteToken(jsontext.String("baz")))

	bufErr(enc.WriteToken(jsontext.BeginArray))
	assertDepth(enc, 2)

	bufErr(enc.WriteToken(jsontext.String("qux")))
	bufErr(enc.WriteToken(jsontext.Int(123)))
	bufErr(enc.WriteToken(jsontext.String("quux")))
	if enc.OutputOffset() == int64(buf.Len()) {
		t.Errorf("immediately flushed at %d", enc.OutputOffset())
	}
	v := enc.UnusedBuffer()
	v = append(v, []byte(`[`)...)
	v = append(v, []byte(`{"corge":null}`)...)
	v = append(v, []byte(`]`)...)
	assertDepth(enc, 2)
	bufErr(enc.WriteValue(v))
	assertDepth(enc, 2)

	t.Log(enc.StackIndex(0)) // encoder_test.go:52: <invalid jsontext.Kind: '\x00'> 1
	t.Log(enc.StackIndex(1)) // encoder_test.go:53: { 4
	t.Log(enc.StackIndex(2)) // encoder_test.go:54: [ 4

	bufErr(enc.WriteToken(jsontext.EndArray))
	assertDepth(enc, 1)
	bufErr(enc.WriteToken(jsontext.EndObject))
	assertDepth(enc, 0)

	if err != nil {
		panic(err)
	}
	expected := `{
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
	if buf.String() != expected {
		t.Fatalf("not equal:\nexpected = %s\nactual  = %s", expected, buf.String())
	}
}
