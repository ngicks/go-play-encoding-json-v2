package play

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"sync"
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
	eitherErr := func(errL, errR error) error {
		return fmt.Errorf("Either[L, R]: unmarshal failed for both L and R: l = (%w), r = (%w)", errL, errR)
	}
	switch dec.PeekKind() {
	case 'n', 'f', 't', '"', '0':
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

		return eitherErr(errL, errR)
	case '{', '[': // maybe deep and large
		var wg sync.WaitGroup
		defer wg.Wait() // in case of panic

		prl, pwl := io.Pipe()
		prr, pwr := io.Pipe()
		defer func() {
			prl.Close()
			prr.Close()
		}()

		var (
			panicVal  any
			storeOnce sync.Once
		)
		recoverStoreOnce := func() {
			if rec := recover(); rec != nil {
				storeOnce.Do(func() {
					panicVal = rec
				})
			}
		}

		errUnmarshalFailedEarly := errors.New("unmarshal failed early")
		wg.Add(1)
		go func() {
			var err error
			defer func() {
				recoverStoreOnce()
				pwl.CloseWithError(err)
				pwr.CloseWithError(err)
				wg.Done()
			}()

			encl := jsontext.NewEncoder(pwl)
			encr := jsontext.NewEncoder(pwr)

			depth := dec.StackDepth()
			for {
				var tok jsontext.Token
				tok, err = dec.ReadToken()
				if err != nil {
					return
				}

				errL := encl.WriteToken(tok)
				if errL != nil && !errors.Is(errL, errUnmarshalFailedEarly) {
					err = errL
					return
				}
				errR := encr.WriteToken(tok)
				if errR != nil && !errors.Is(errR, errUnmarshalFailedEarly) {
					err = errR
					return
				}
				if errL != nil && errR != nil {
					err = errUnmarshalFailedEarly
					return
				}

				if dec.StackDepth() == depth {
					break
				}
			}
		}()

		var (
			l          L
			r          R
			errL, errR error
		)

		wg.Add(1)
		go func() {
			defer func() {
				recoverStoreOnce()
				wg.Done()
			}()
			errL = json.UnmarshalRead(prl, &l, dec.Options())
			if errL != nil { // successful = tokens are fully consumed
				prl.CloseWithError(errUnmarshalFailedEarly)
			}
		}()

		errR = json.UnmarshalRead(prr, &r, dec.Options())
		if errR != nil {
			prr.CloseWithError(errUnmarshalFailedEarly)
		}

		wg.Wait()
		if panicVal != nil {
			panic(panicVal)
		}

		if errL == nil {
			e.isRight = false
			e.l = l
			e.r = *new(R)
			return nil
		}

		if errR == nil {
			e.isRight = true
			e.l = *new(L)
			e.r = r
			return nil
		}

		return eitherErr(errL, errR)
	default: // invalid, '}',	']'
		// syntax error
		_, err := dec.ReadValue()
		return err
	}
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
		{"{\"foo\": false}", true},
	} {
		var e Either[string, int]
		err := json.Unmarshal([]byte(tc.in), &e)
		if (err != nil) != tc.fail {
			t.Errorf("incorrect!")
		}
		t.Logf("err = %v", err)
		/*
		   arshaler_either_test.go:245: err = <nil>
		   arshaler_either_test.go:245: err = <nil>
		   arshaler_either_test.go:245: err = json: cannot unmarshal into Go play.Either[string,int]: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go string), r = (json: cannot unmarshal JSON boolean into Go int)
		   arshaler_either_test.go:245: err = json: cannot unmarshal into Go play.Either[string,int] after offset 13: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON object into Go string), r = (json: cannot unmarshal JSON object into Go int)
		*/
	}

	type sampleL struct {
		Foo []int
	}
	type sampleR struct {
		Bar map[string]string
	}
	for _, tc := range []testCase{
		{"\"foo\"", true},
		{"123", true},
		{"false", true},
		{"{\"foo\": false}", true},
		{"{\"Foo\": false}", true},
		{"{\"Foo\": [1,2,3]}", false},
		{"{\"Bar\": {\"foo\":\"foofoo\",\"bar\":\"barbar\"}}", false},
		{"{\"Foo\": [1,2,3}", true},     // syntax error
		{"{\"Bar\": {\"foo\":}}", true}, // syntax error
	} {
		t.Run(tc.in, func(t *testing.T) {
			var e Either[sampleL, sampleR]
			err := json.Unmarshal([]byte(tc.in), &e, json.RejectUnknownMembers(true))
			if (err != nil) != tc.fail {
				t.Errorf("incorrect!")
			}
			t.Logf("err = %v", err)
			/*
				=== RUN   TestArshalerEither/"foo"
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON string into Go play.sampleL), r = (json: cannot unmarshal JSON string into Go play.sampleR)
				=== RUN   TestArshalerEither/123
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON number into Go play.sampleL), r = (json: cannot unmarshal JSON number into Go play.sampleR)
				=== RUN   TestArshalerEither/false
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go play.sampleL), r = (json: cannot unmarshal JSON boolean into Go play.sampleR)
				=== RUN   TestArshalerEither/{"foo":_false}
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct after offset 13: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON string into Go play.sampleL: unknown object member name "foo"), r = (json: cannot unmarshal JSON string into Go play.sampleR: unknown object member name "foo")
				=== RUN   TestArshalerEither/{"Foo":_false}
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct after offset 13: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go []int within "/Foo"), r = (json: cannot unmarshal JSON string into Go play.sampleR: unknown object member name "Foo")
				=== RUN   TestArshalerEither/{"Foo":_[1,2,3]}
				    arshaler_either_test.go:277: err = <nil>
				=== RUN   TestArshalerEither/{"Bar":_{"foo":"foofoo","bar":"barbar"}}
				    arshaler_either_test.go:277: err = <nil>
				=== RUN   TestArshalerEither/{"Foo":_[1,2,3}
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct within "/Foo": Either[L, R]: unmarshal failed for both L and R: l = (jsontext: read error: jsontext: invalid character '}' after array element (expecting ',' or ']') within "/Foo" after offset 14), r = (json: cannot unmarshal JSON string into Go play.sampleR: unknown object member name "Foo")
				=== RUN   TestArshalerEither/{"Bar":_{"foo":}}
				    arshaler_either_test.go:277: err = json: cannot unmarshal into Go struct within "/Bar/foo": Either[L, R]: unmarshal failed for both L and R: l = (jsontext: read error: jsontext: missing value after object name within "/Bar/foo" after offset 15), r = (jsontext: read error: jsontext: missing value after object name within "/Bar/foo" after offset 15)
			*/
		})
	}
}
