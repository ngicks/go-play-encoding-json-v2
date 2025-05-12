package play

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

func ReadJSONAt(dec *jsontext.Decoder, pointer jsontext.Pointer, read func(dec *jsontext.Decoder) error) (err error) {
	lastToken := pointer.LastToken()
	var idx int64 = -1
	if len(lastToken) > 0 && strings.TrimLeftFunc(lastToken, func(r rune) bool { return '0' <= r && r <= '9' }) == "" {
		idx, err = strconv.ParseInt(lastToken, 10, 64)
		if err == nil {
			pointer = pointer[:len(pointer)-len(lastToken)-1]
		} else {
			// I'm not really super sure this could happen.
			idx = -1
		}
	}

	for {
		_, err = dec.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		p := dec.StackPointer()
		if pointer == p {
			if idx >= 0 {
				// skip '['
				_, err = dec.ReadToken()
				if err != nil {
					return err
				}
				for ; idx > 0; idx-- {
					err := dec.SkipValue()
					if err != nil {
						return err
					}
				}
			}
			return read(dec)
		}
	}
	return nil
}

func TestDecoder_Pointer(t *testing.T) {
	jsonBuf := []byte(`{"yay":"yay","nay":[{"boo":"boo"},{"bobo":"bobo"}],"foo":{"bar":{"baz":"baz"}}}`)

	type Boo struct {
		Boo string `json:"boo"`
	}
	type Bobo struct {
		Bobo string `json:"bobo"`
	}
	type Baz struct {
		Baz string `json:"baz"`
	}

	type testCase struct {
		pointer    jsontext.Pointer
		readTarget any
		expected   any
	}
	for _, tc := range []testCase{
		{"/foo/bar/baz/qux", nil, nil},
		{"/foo/bar", Baz{}, Baz{"baz"}},
		{"/nay/0", Boo{}, Boo{"boo"}},
		{"/nay/1", Bobo{}, Bobo{"bobo"}},
	} {
		found := false
		err := ReadJSONAt(
			jsontext.NewDecoder(bytes.NewBuffer(jsonBuf)),
			tc.pointer,
			func(dec *jsontext.Decoder) error {
				found = true
				return json.UnmarshalDecode(dec, &tc.readTarget)
			},
		)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if !found {
			if tc.readTarget != nil {
				t.Errorf("not found: expected = %#v", tc.expected)
			}
		} else {
			expected := fmt.Sprintf("%#v", tc.expected)
			read := fmt.Sprintf("%#v", tc.readTarget)
			if expected != read {
				t.Errorf("read not as expected: expected(%q) != actual(%q)", expected, read)
			}
		}
	}
}
