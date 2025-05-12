package play

import (
	"encoding/json/v2"
	"testing"
	"time"
)

func TestTagFormat(t *testing.T) {
	type sample struct {
		Foo map[string]string `json:",format:emitempty"`
		Bar []byte            `json:",format:array"`
		Baz time.Duration     `json:",format:units"`
		Qux time.Time         `json:",format:'2006-01-02'"`
	}

	s := sample{
		Foo: nil,
		Bar: []byte(`bar`),
		Baz: time.Minute,
		Qux: time.Date(2025, 0o5, 12, 22, 23, 22, 123456789, time.UTC),
	}

	bin, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	expected := `{"Foo":{},"Bar":[98,97,114],"Baz":"1m0s","Qux":"2025-05-12"}`
	if string(bin) != expected {
		t.Errorf("not equal:\n%s\n!=\n%s", expected, string(bin))
	}
}
