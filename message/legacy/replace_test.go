// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/kylelemons/godebug/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSignature(t *testing.T) {
	r := require.New(t)
	var input = []byte(`{"foo":"test","signature":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==.sig.ed25519"}`)
	enc, err := PrettyPrint(input)
	r.NoError(err, "encode failed")

	_, sign, err := ExtractSignature(enc)
	r.NoError(err, "extract sig failed")
	r.NotNil(sign)
	r.True(bytes.Equal(sign, bytes.Repeat([]byte{0}, 64)))
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
	if s := matches[1]; !bytes.Equal(s, wantSig) {
		t.Errorf("unexpected submatch: %s", s)
	}
	out := signatureRegexp.ReplaceAll(input, []byte{})
	if !bytes.Equal(out, want) {
		t.Errorf("got unexpected replace:\n%s", out)
	}
}

func TestUnicodeFind(t *testing.T) {
	in := "Hello\x01World"
	want := `Hello\u0001World`
	out := unicodeEscapeSome(in)
	assert.Equal(t, want, out)
}

// see below
// func getHexBytesFromNode(t *testing.T, input, encoding string) []byte {
// 	cmd := exec.Command("node", "-e", fmt.Sprintf(`console.log(new Buffer("%s", "%s"))`, input, encoding))
// 	out, err := cmd.CombinedOutput()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	out = bytes.TrimPrefix(out, []byte("<Buffer "))
// 	out = bytes.TrimSuffix(out, []byte(">\n"))
// 	out = bytes.Replace(out, []byte(" "), []byte{}, -1)
// 	t.Logf(" %s:\t%s", encoding, string(out))
// 	return out
// }

func TestInternalV8String(t *testing.T) {
	r := require.New(t)
	type tcase struct {
		in, want string
	}
	testStrs := []tcase{
		{"foo", "666f6f"},
		{"···", "b7b7b7"},
		{"Fabián", "46616269e16e"},
		{"üäá", "fce4e1"},
		{"“SaneScript”", "1c53616e655363726970741d"},
		{"🄯řÿþŧį×", "3c2f59fffe672fd7"},
		// add more examples as needed
	}
	for _, v := range testStrs {
		/* might to regnerate your assumptions?
		u8 := getHexBytesFromNode(t, v, "utf8")
		bin := getHexBytesFromNode(t, v, "binary")
		r.Equal(fmt.Sprintf("%x", v), string(u8), "assuming we are dealing with utf8 on our side")
		*/

		got, err := InternalV8Binary([]byte(v.in))
		r.NoError(err)
		p := fmt.Sprintf("%x", got)

		if d := diff.Diff(v.want, p); len(d) != 0 {
			t.Logf("\n%s", d)
		}
	}
}
