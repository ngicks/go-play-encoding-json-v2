package play

import (
	"encoding/json/v2"
	"maps"
	"testing"
)

func TestTagUnknown(t *testing.T) {
	type sample struct {
		X   map[string]any `json:",unknown"`
		Foo string
		Bar int
		Baz bool
	}

	input := []byte(`{"Foo":"foo","Bar":12,"Baz":true,"Qux":"qux","Quux":"what!?"}`)
	var s sample
	err := json.Unmarshal(input, &s)
	if err != nil {
		panic(err)
	}
	expected := map[string]any{"Qux": "qux", "Quux": "what!?"}
	if !maps.Equal(s.X, expected) {
		t.Errorf("not equal:\n%#v\n!=\n%#v", expected, s.X)
	}

	err = json.Unmarshal(input, &s, json.RejectUnknownMembers(true))
	if err == nil {
		t.Error("should cause an error")
	} else {
		t.Logf("%v", err)
		// tag_unknown_test.go:32: json: cannot unmarshal JSON string into Go play.sample: unknown object member name "Qux"
	}
}
