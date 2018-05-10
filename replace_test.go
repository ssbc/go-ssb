package ssb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSignature(t *testing.T) {
	var input = []byte(`{"foo":"test","signature":"testSign"}`)
	_, sign, err := EncodePreserveOrder(input)
	if err != nil {
		t.Fatal(err)
	}
	if sign != "testSign" {
		t.Errorf("got unexpected signature: %s", sign)
	}
}

func TestStripSignature(t *testing.T) {
	var (
		input = []byte(`{
  "foo": "hello",
  "signature": "aBISzGroszUndKlein01234567890/+="
}`)
		want = []byte(`{
  "foo": "hello"
}`)
		wantSig = []byte("aBISzGroszUndKlein01234567890/+=")
	)
	matches := signatureRegexp.FindSubmatch(input)
	if n := len(matches); n != 2 {
		t.Fatalf("expected 2 results, got %d", n)
	}
	if s := matches[1]; bytes.Compare(s, wantSig) != 0 {
		t.Errorf("unexpected submatch: %s", s)
	}
	out := signatureRegexp.ReplaceAll(input, []byte{})
	if bytes.Compare(out, want) != 0 {
		t.Errorf("got unexpected replace:\n%s", out)
	}
}

func TestUnicodeFind(t *testing.T) {
	in := "Hello\x01World"
	want := `Hello\u0001World`
	out := unicodeEscapeSome(in)
	assert.Equal(t, want, out)
}
