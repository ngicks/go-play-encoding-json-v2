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
	U   string    `json:"',\"'"`
}

func main() {
	a := A{
		Foo: "foo",
		Bar: 123,
		T:   time.Now(),
		U:   "um",
	}

	bin, err := json.Marshal(&a, jsontext.WithIndent("    "))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bin))
	/*
	   {
	       "foo": "foo",
	       "int": 123,
	       "t": "2025-05-23T21:47:23+09:00",
	       ",\"": "um"
	   }
	*/
	a = *new(A)
	err = json.Unmarshal(bin, &a)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%#v\n", a)
	// main.A{Foo:"foo", Bar:123, T:time.Date(2025, time.May, 23, 21, 47, 23, 0, time.Local), U:"um"}
}
