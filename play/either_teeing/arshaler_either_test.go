package play

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"
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

type ReadCloseStopper interface {
	io.ReadCloser
	Stop(successful bool) // when called, stops both side of tee.
}

type bufReader struct {
	mu     sync.Mutex
	closed bool
	r      *bytes.Reader
}

func (r *bufReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, io.EOF
	}
	return r.r.Read(p)
}

func (r *bufReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

func (r *bufReader) Stop(successful bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

var (
	errStopped     = errors.New("stopped")
	errFailedEarly = errors.New("failed early")
)

type panicErr struct{ v any }

func (e *panicErr) Error() string {
	return fmt.Sprintf("panicked: %v", e.v)
}

type teeReader struct {
	mu     sync.Mutex
	closed bool
	r      *io.PipeReader
}

func (r *teeReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, io.EOF
	}

	n, err := r.r.Read(p)

	var pe *panicErr
	if errors.As(err, &pe) {
		panic(pe.v)
	}

	return n, err
}

func (r *teeReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	// one last read
	var p [1]byte
	_, err := r.r.Read(p[:])
	var pe *panicErr
	if errors.As(err, &pe) {
		panic(pe.v)
	}

	return r.r.Close()
}

func (r *teeReader) Stop(successful bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}
	r.closed = true

	err := errFailedEarly
	if successful {
		err = errStopped
	}
	r.r.CloseWithError(err)
}

type multiPipeWriter struct {
	maskedErr error
	wl, wr    *io.PipeWriter
}

func (w *multiPipeWriter) Write(b []byte) (n int, err error) {
	if w.wl != nil {
		var nl int
		nl, err = w.wl.Write(b)
		if err != nil {
			// failed write = the other side of pipe is closed with error or anything.
			w.wl = nil
			if !errors.Is(err, w.maskedErr) {
				w.wr.CloseWithError(err)
				w.wr = nil
				return
			}
		} else {
			n = nl
		}
		err = nil
	}

	if w.wr != nil {
		var nr int
		nr, err = w.wr.Write(b)
		if err != nil {
			w.wr = nil
			if !errors.Is(err, w.maskedErr) {
				w.wl.CloseWithError(err)
				w.wl = nil
				return
			}
		} else {
			n = nr
		}
		err = nil
	}
	if len(b) != n && err == nil {
		err = io.ErrClosedPipe
	}
	return
}

func (w *multiPipeWriter) CloseWithError(err error) error {
	if w.wl != nil {
		w.wl.CloseWithError(err)
		w.wl = nil
	}
	if w.wr != nil {
		w.wr.CloseWithError(err)
		w.wr = nil
	}
	return nil
}

var errBadDec = errors.New("bad decoder")

func TeeDecoder(dec *jsontext.Decoder, encOptions ...jsontext.Options) (l ReadCloseStopper, r ReadCloseStopper, wait func(), err error) {
	switch dec.PeekKind() {
	default:
		return nil, nil, func() {}, fmt.Errorf("%w: decoder peeked a non starting token %q", errBadDec, dec.PeekKind().String())
	case 'n', 'f', 't', '"', '0':
		val, err := dec.ReadValue()
		if err != nil {
			return nil, nil, func() {}, err
		}
		return &bufReader{r: bytes.NewReader(val)}, &bufReader{r: bytes.NewReader(val)}, func() {}, nil
	case '[', '{':
		prl, pwl := io.Pipe()
		prr, pwr := io.Pipe()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			mw := &multiPipeWriter{errFailedEarly, pwl, pwr}
			defer func() {
				// it's possible that reading dec panicks
				if rec := recover(); rec != nil {
					err = &panicErr{rec}
				}
				mw.CloseWithError(err)
			}()

			enc := jsontext.NewEncoder(mw, encOptions...)

			depth := dec.StackDepth()
			var tok jsontext.Token
			for {
				tok, err = dec.ReadToken()
				if err != nil {
					return
				}

				err = enc.WriteToken(tok)
				if err != nil {
					return
				}

				if dec.StackDepth() == depth {
					break
				}
			}
		}()

		return &teeReader{r: prl}, &teeReader{r: prr}, wg.Wait, nil
	}
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

		var err error

		rl, rr, wait, err := TeeDecoder(dec)
		if err != nil {
			return err
		}
		defer func() {
			rl.Stop(false)
			rr.Stop(false)
			wait()
		}()

		var (
			panicVal  any
			storeOnce sync.Once
		)
		var (
			l          L
			r          R
			errL, errR error
		)

		wg.Add(1)
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					storeOnce.Do(func() {
						panicVal = rec
					})
				}
				rr.Stop(false)
				wg.Done()
			}()
			errR = json.UnmarshalRead(rr, &r, dec.Options())
		}()

		errL = json.UnmarshalRead(rl, &l, dec.Options())
		rl.Stop(errL == nil)

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
		t.Run(tc.in, func(t *testing.T) {
			var e Either[string, int]
			err := json.Unmarshal([]byte(tc.in), &e)
			if (err != nil) != tc.fail {
				t.Errorf("incorrect!")
			}
			t.Logf("err = %v", err)
			/*
			   === RUN   TestArshalerEither
			   === RUN   TestArshalerEither/"foo"
			       arshaler_either_test.go:418: err = <nil>
			   === RUN   TestArshalerEither/123
			       arshaler_either_test.go:418: err = <nil>
			   === RUN   TestArshalerEither/false
			       arshaler_either_test.go:418: err = json: cannot unmarshal into Go play.Either[string,int]: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go string), r = (json: cannot unmarshal JSON boolean into Go int)
			   === RUN   TestArshalerEither/{"foo":_false}
			       arshaler_either_test.go:418: err = json: cannot unmarshal into Go play.Either[string,int] after offset 13: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON object into Go string), r = (json: cannot unmarshal JSON object into Go int)
			*/
		})
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
			   === RUN   TestArshalerEither/"foo"#01
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON string into Go play.sampleL), r = (json: cannot unmarshal JSON string into Go play.sampleR)
			   === RUN   TestArshalerEither/123#01
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON number into Go play.sampleL), r = (json: cannot unmarshal JSON number into Go play.sampleR)
			   === RUN   TestArshalerEither/false#01
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go play.sampleL), r = (json: cannot unmarshal JSON boolean into Go play.sampleR)
			   === RUN   TestArshalerEither/{"foo":_false}#01
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct after offset 13: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON string into Go play.sampleL: unknown object member name "foo"), r = (json: cannot unmarshal JSON string into Go play.sampleR: unknown object member name "foo")
			   === RUN   TestArshalerEither/{"Foo":_false}
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct after offset 13: Either[L, R]: unmarshal failed for both L and R: l = (json: cannot unmarshal JSON boolean into Go []int within "/Foo"), r = (json: cannot unmarshal JSON string into Go play.sampleR: unknown object member name "Foo")
			   === RUN   TestArshalerEither/{"Foo":_[1,2,3]}
			       arshaler_either_test.go:456: err = <nil>
			   === RUN   TestArshalerEither/{"Bar":_{"foo":"foofoo","bar":"barbar"}}
			       arshaler_either_test.go:456: err = <nil>
			   === RUN   TestArshalerEither/{"Foo":_[1,2,3}
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct within "/Foo": Either[L, R]: unmarshal failed for both L and R: l = (jsontext: read error: jsontext: invalid character '}' after array element (expecting ',' or ']') within "/Foo" after offset 14), r = (json: cannot unmarshal JSON string into Go play.sampleR: unknown object member name "Foo")
			   === RUN   TestArshalerEither/{"Bar":_{"foo":}}
			       arshaler_either_test.go:456: err = json: cannot unmarshal into Go struct within "/Bar/foo": Either[L, R]: unmarshal failed for both L and R: l = (jsontext: read error: jsontext: missing value after object name within "/Bar/foo" after offset 15), r = (jsontext: read error: jsontext: missing value after object name within "/Bar/foo" after offset 15)
			*/
		})
	}
}

var panicVal any = "panicVal"

type panicReader struct {
	after io.Reader
	val   any
}

func (r *panicReader) Read(p []byte) (int, error) {
	n, err := r.after.Read(p)
	if err == io.EOF {
		panic(r.val)
	}
	return n, err
}

var _ json.UnmarshalerFrom = (*panicDecoder)(nil)

type panicDecoder struct{}

func (d *panicDecoder) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	panic(panicVal)
}

func TestArshalerEither_panic(t *testing.T) {
	t.Run("reader panics", func(t *testing.T) {
		defer func() {
			rec := recover()
			if rec != panicVal {
				t.Errorf("incorrect panic: want(%v) != got(%v)", panicVal, rec)
			}
		}()
		var e Either[map[int]any, map[string]any]
		json.UnmarshalRead(&panicReader{strings.NewReader(`{"foo":"foo",`), panicVal}, &e)
	})
	t.Run("left panics", func(t *testing.T) {
		defer func() {
			rec := recover()
			if rec != panicVal {
				t.Errorf("incorrect panic: want(%v) != got(%v)", panicVal, rec)
			}
		}()
		var e Either[panicDecoder, map[string]any]
		json.Unmarshal([]byte(`{"foo":"foo","bar":"bar"}`), &e)
	})
	t.Run("right panics", func(t *testing.T) {
		defer func() {
			rec := recover()
			if rec != panicVal {
				t.Errorf("incorrect panic: want(%v) != got(%v)", panicVal, rec)
			}
		}()
		var e Either[map[string]any, panicDecoder]
		json.Unmarshal([]byte(`{"foo":"foo","bar":"bar"}`), &e)
	})
}
