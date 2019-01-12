package ssb

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/keks/nocomment"
	"github.com/pkg/errors"
	"go.cryptoscope.co/secretstream/secrethandshake"
)

type KeyPair struct {
	Id   *FeedRef
	Pair secrethandshake.EdKeyPair
}

// the format of the .ssb/secret file as defined by the js implementations
var sbotKey struct {
	Curve   string   `json:"curve"`
	ID      *FeedRef `json:"id"`
	Private string   `json:"private"`
	Public  string   `json:"public"`
}

// LoadKeyPair opens fname, ignores any line starting with # and passes it ParseKeyPair
func LoadKeyPair(fname string) (*KeyPair, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.LoadKeyPair: could not open key file %s", fname)
	}
	defer f.Close()

	return ParseKeyPair(nocomment.NewReader(f))
}

// ParseKeyPair json decodes an object from the reader.
// It expects std base64 encoded data under the `private` and `public` fields.
func ParseKeyPair(r io.Reader) (*KeyPair, error) {
	if err := json.NewDecoder(r).Decode(&sbotKey); err != nil {
		return nil, errors.Wrapf(err, "ssb.Parse: JSON decoding failed")
	}

	public, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(sbotKey.Public, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.Parse: base64 decode of public part failed")
	}

	private, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(sbotKey.Private, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.Parse: base64 decode of private part failed")
	}

	var kp secrethandshake.EdKeyPair
	copy(kp.Public[:], public)
	copy(kp.Secret[:], private)

	ssbkp := KeyPair{
		Id:   sbotKey.ID,
		Pair: kp,
	}
	return &ssbkp, nil
}
