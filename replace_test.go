package ssb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindAuthor(t *testing.T) {
	a, r := assert.New(t), require.New(t)
	tMsg := testMessages[1]

	enc, err := EncodePreserveOrder(tMsg.Input)
	r.NoError(err, "encode failed")

	matches := authorRegexp.FindSubmatch(enc)
	r.Len(matches, 2)
	a.Equal(string(matches[1]), tMsg.Author.Ref())
}

func TestExtractSignature(t *testing.T) {
	r := require.New(t)
	var input = []byte(`{"foo":"test","signature":"testSign"}`)
	enc, err := EncodePreserveOrder(input)
	r.NoError(err, "encode failed")

	_, sign, err := ExtractSignature(enc)
	r.NoError(err, "extract sig failed")
	r.NotNil(sign)
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
