package play

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"testing"
)

var (
	_ json.MarshalerTo     = Either[any, any]{}
	_ json.UnmarshalerFrom = (*Either[any, any])(nil)
)

// zero value is zero left.
type Either[L, R any] struct {
	isRight bool
	l       L
	r       R
}

func Left[L, R any](l L) Either[L, R] {
	return Either[L, R]{isRight: false, l: l}
}

func Right[L, R any](r R) Either[L, R] {
	return Either[L, R]{isRight: true, r: r}
}

func (e Either[L, R]) IsLeft() bool {
	return !e.isRight
}

func (e Either[L, R]) IsRight() bool {
	return e.isRight
}

func (e Either[L, R]) Left() L {
	return e.l
}

func (e Either[L, R]) Right() R {
	return e.r
}

func (e Either[L, R]) Unpack() (L, R) {
	// for ? syntax discussed under https://github.com/golang/go/discussions/71460
	return e.l, e.r
}

func MapLeft[L, R, L2 any](e Either[L, R], mapper func(l L) L2) Either[L2, R] {
	if e.IsLeft() {
		return Left[L2, R](mapper(e.Left()))
	}
	return Right[L2](e.Right())
}

func (e Either[L, R]) MapLeft(mapper func(l L) L) Either[L, R] {
	return MapLeft(e, mapper)
}

func MapRight[L, R, R2 any](e Either[L, R], mapper func(l R) R2) Either[L, R2] {
	if e.IsRight() {
		return Right[L](mapper(e.Right()))
	}
	return Left[L, R2](e.Left())
}

func (e Either[L, R]) MapRight(mapper func(l R) R) Either[L, R] {
	return MapRight(e, mapper)
}

func (e Either[L, R]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if e.IsLeft() {
		return json.MarshalEncode(enc, e.Left())
	}
	return json.MarshalEncode(enc, e.Right())
}

func (e *Either[L, R]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	val, err := dec.ReadValue()
	if err != nil {
		return err
	}

	var l L
	errL := json.Unmarshal(val, &l, dec.Options())
	if errL == nil {
		e.isRight = false
		e.l = l
		e.r = *new(R)
		return nil
	}

	var r R
	errR := json.Unmarshal(val, &r, dec.Options())
	if errR == nil {
		e.isRight = true
		e.l = *new(L)
		e.r = r
		return nil
	}

	return fmt.Errorf("Either[L, R]: unmarshal failed for both L and R: l = (%w), r = (%w)", errL, errR)
}

func TestArshalerEither(t *testing.T) {
	type testCase struct {
		in   string
		fail bool
	}
	for _, tc := range []testCase{
		{"\"foo\"", false},
		{"123", false},
		{"false", true},
	} {
		var e Either[string, int]
		err := json.Unmarshal([]byte(tc.in), &e)
		if (err != nil) != tc.fail {
			t.Errorf("incorrect!")
		}
		t.Logf("err = %v", err)
		/*
		   arshaler_either_test.go:122: err = <nil>
		   arshaler_either_test.go:122: err = <nil>
		   arshaler_either_test.go:122: err = json: cannot unmarshal into Go play.Either[string,int]: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go string), r = (json: cannot unmarshal JSON boolean into Go int)
		*/
	}
}
