package play

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"
)

var (
	_ json.MarshalerTo     = Option[any]{}
	_ json.UnmarshalerFrom = (*Option[any])(nil)
	_ json.MarshalerTo     = Und[any]{}
	_ json.UnmarshalerFrom = (*Und[any])(nil)
)

type Option[V any] struct {
	some bool
	v    V
}

func None[V any]() Option[V] {
	return Option[V]{}
}

func Some[V any](v V) Option[V] {
	return Option[V]{some: true, v: v}
}

func (o Option[V]) IsZero() bool {
	return o.IsNone()
}

func (o Option[V]) IsNone() bool {
	return !o.some
}

func (o Option[V]) IsSome() bool {
	return o.some
}

func (o Option[V]) Value() V {
	return o.v
}

func (o Option[V]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if o.IsNone() {
		return enc.WriteToken(jsontext.Null)
	}
	return json.MarshalEncode(enc, o.Value())
}

func (o *Option[V]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if k := dec.PeekKind(); k == 'n' {
		err := dec.SkipValue()
		if err != nil {
			return err
		}
		o.some = false
		o.v = *new(V)
		return nil
	}
	var v V
	err := json.UnmarshalDecode(dec, &v)
	if err != nil {
		return err
	}
	// preventing half-baked value left in-case of error in middle of decode
	// sacrificing performance
	o.some = true
	o.v = v
	return nil
}

type Und[V any] struct {
	opt Option[Option[V]]
}

func Undefined[V any]() Und[V] {
	return Und[V]{}
}

func Null[V any]() Und[V] {
	return Und[V]{opt: Some(None[V]())}
}

func Defined[V any](v V) Und[V] {
	return Und[V]{opt: Some(Some(v))}
}

func (u Und[V]) IsZero() bool {
	return u.IsUndefined()
}

func (u Und[V]) IsUndefined() bool {
	return u.opt.IsNone()
}

func (u Und[V]) IsNull() bool {
	return u.opt.IsSome() && u.opt.Value().IsNone()
}

func (u Und[V]) IsDefined() bool {
	return u.opt.IsSome() && u.opt.Value().IsSome()
}

func (u Und[V]) Value() V {
	return u.opt.Value().Value()
}

func (u Und[V]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if !u.IsDefined() {
		return enc.WriteToken(jsontext.Null)
	}
	return json.MarshalEncode(enc, u.Value())
}

func (u *Und[V]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	// should be with omitzero which handles absence of field.
	if k := dec.PeekKind(); k == 'n' {
		err := dec.SkipValue()
		if err != nil {
			return err
		}
		*u = Null[V]()
		return nil
	}
	var v V
	err := json.UnmarshalDecode(dec, &v)
	if err != nil {
		return err
	}
	*u = Defined(v)
	return nil
}

func TestArshalerToFrom(t *testing.T) {
	type sample struct {
		// null or string
		Foo Option[string]
		// undefined or string
		Bar Option[int] `json:",omitzero"`
		// undefined | null | bool
		Baz Und[bool] `json:",omitzero"`
	}

	type testCase struct {
		in        sample
		marshaled string
	}
	for _, tc := range []testCase{
		{sample{}, `{"Foo":null}`},
		{sample{Some(""), Some(0), Null[bool]()}, `{"Foo":"","Bar":0,"Baz":null}`},
		{sample{Some("foo"), Some(5), Defined(false)}, `{"Foo":"foo","Bar":5,"Baz":false}`},
		{sample{None[string](), None[int](), Defined(true)}, `{"Foo":null,"Baz":true}`},
	} {
		t.Run(tc.marshaled, func(t *testing.T) {
			bin, err := json.Marshal(tc.in)
			if err != nil {
				panic(err)
			}
			if string(bin) != tc.marshaled {
				t.Errorf("not equal: expected(%q) != actual(%q)", tc.marshaled, string(bin))
			}
			var unmarshaled sample
			err = json.Unmarshal(bin, &unmarshaled)
			if err != nil {
				panic(err)
			}
			if unmarshaled != tc.in {
				t.Errorf("not euql:\nexpected(%#v)\n!=\nactual(%#v)", tc.in, unmarshaled)
			}
		})
	}
}
