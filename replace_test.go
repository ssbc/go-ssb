package ssb

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"

	"github.com/catherinejones/testdiff"
	"github.com/kylelemons/godebug/diff"
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

func getHexBytesFromNode(t *testing.T, input, encoding string) []byte {
	cmd := exec.Command("node", "-e", fmt.Sprintf(`console.log(new Buffer("%s", "%s"))`, input, encoding))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(" %s:\t%s", encoding, string(out))

	out = bytes.TrimPrefix(out, []byte("<Buffer "))
	out = bytes.TrimSuffix(out, []byte(">\n"))
	out = bytes.Replace(out, []byte(" "), []byte{}, -1)
	return out
}

func TestInternalV8String(t *testing.T) {
	r := require.New(t)
	testStrs := []string{
		"foo",
		"···",
		"Fabián",
		"üäá",
		"“SaneScript”",
		"🄯řÿþŧį×",
		// add more examples as needed
	}
	for i, v := range testStrs {
		t.Logf("%02d: %s", i, v)
		// todo: don't actually need node here.. :S
		u8 := getHexBytesFromNode(t, v, "utf8")
		bin := getHexBytesFromNode(t, v, "binary")
		r.Equal(fmt.Sprintf("%x", v), string(u8), "assuming we are dealing with utf8 on our side")

		want := string(bin)

		got, err := internalV8Binary([]byte(v))
		r.NoError(err)
		p := fmt.Sprintf("%x", got)

		testdiff.StringIs(t, want, p)
		if d := diff.Diff(want, p); len(d) != 0 {
			t.Logf("\n%s", d)
		}
	}
}
