package sbot

import (
	"encoding/base64"
	"encoding/json"
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

func LoadKeyPair(fname string) (*KeyPair, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.LoadKeyPair: could not open key file")
	}
	defer f.Close()

	var sbotKey struct {
		Curve   string   `json:"curve"`
		ID      *FeedRef `json:"id"`
		Private string   `json:"private"`
		Public  string   `json:"public"`
	}

	if err := json.NewDecoder(nocomment.NewReader(f)).Decode(&sbotKey); err != nil {
		return nil, errors.Wrapf(err, "ssb.LoadKeyPair: json decoding of %q failed.", fname)
	}

	public, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(sbotKey.Public, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.LoadKeyPair: base64 decode of public part failed.")
	}

	private, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(sbotKey.Private, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.LoadKeyPair: base64 decode of private part failed.")
	}

	var kp secrethandshake.EdKeyPair
	copy(kp.Public[:], public)
	copy(kp.Secret[:], private)

	return &KeyPair{
		Id:   sbotKey.ID,
		Pair: kp,
	}, nil
}
