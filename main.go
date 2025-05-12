package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"time"
)

type A struct {
	Foo string    `json:"foo,omitzero"`
	Bar int       `json:"int,omitzero"`
	T   time.Time `json:"t,omitzero,format:RFC3339"`
}

func main() {
	a := A{
		Foo: "foo",
		Bar: 123,
		T:   time.Now(),
	}

	bin, err := json.Marshal(a, jsontext.WithIndent("    "))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bin))
}
