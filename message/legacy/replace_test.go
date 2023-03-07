// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"fmt"
	"os/exec"
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
  "signature": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==.sig.ed25519"
}`)
		want = []byte(`{
  "foo": "hello"
}`)
	)

	wantSig, err := NewSignatureFromBase64([]byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==.sig.ed25519"))
	require.NoError(t, err)

	msg, sig, err := ExtractSignature(input)
	require.NoError(t, err)

	require.Equal(t, want, msg)
	require.Equal(t, wantSig, sig)
}

func TestUnicodeFind(t *testing.T) {
	in := "Hello\x01World"
	want := `Hello\u0001World`
	buf := &bytes.Buffer{}
	unicodeEscapeSome(buf, in)
	assert.Equal(t, want, buf.String())
}

func getHexBytesFromNode(t *testing.T, input, encoding string) []byte {
	cmd := exec.Command("node", "-e", fmt.Sprintf(`console.log(new Buffer("%s", "%s"))`, input, encoding))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	out = bytes.TrimPrefix(out, []byte("<Buffer "))
	out = bytes.TrimSuffix(out, []byte(">\n"))
	out = bytes.Replace(out, []byte(" "), []byte{}, -1)
	t.Logf(" %s:\t%s", encoding, string(out))
	return out
}

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
