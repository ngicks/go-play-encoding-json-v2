package play

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"reflect"
	"testing"
)

const streamDecodeInput = `{
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

func TestStreamingDecode(t *testing.T) {
	dec := jsontext.NewDecoder(bytes.NewReader([]byte(streamDecodeInput)))
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
	// discard '['
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

func TestStreamingDecode2(t *testing.T) {
	type Data struct {
		Foo string `json:"foo"`
	}

	type Bar struct {
		Baz []Data `json:"baz"`
	}

	type sample struct {
		Foo *int `json:"foo"`
		Bar Bar  `json:"bar"`
	}

	dataChan := make(chan Data)
	unmarshaler := json.WithUnmarshalers(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, d *Data) error {
		type plain Data
		var p plain
		err := json.UnmarshalDecode(dec, &p)
		if err == nil {
			dataChan <- Data(p)
		}
		*d = Data(p)
		return err
	}))

	resultCh := make(chan []Data)
	go func() {
		var result []Data
		for d := range dataChan {
			result = append(result, d)
		}
		resultCh <- result
	}()

	var s sample
	err := json.Unmarshal([]byte(streamDecodeInput), &s, unmarshaler)
	if err != nil {
		panic(err)
	}
	close(dataChan)
	result := <-resultCh

	expected := []Data{{"foo1"}, {"foo2"}, {"foo3"}}
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("not equal:\nexpected(%#v)\n!=\nactual(%#v)", expected, result)
	} else {
		t.Logf("decoded = %#v", result)
		// streaming_decode_test.go:111: decoded = []play.Data{play.Data{Foo:"foo1"}, play.Data{Foo:"foo2"}, play.Data{Foo:"foo3"}}
	}
}
