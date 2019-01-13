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
type ssbSecret struct {
	Curve   string   `json:"curve"`
	ID      *FeedRef `json:"id"`
	Private string   `json:"private"`
	Public  string   `json:"public"`
}

func NewKeyPair(r io.Reader) (*KeyPair, error) {

	// generate new keypair
	kp, err := secrethandshake.GenEdKeyPair(r)
	if err != nil {
		return nil, errors.Wrap(err, "ssb: error building key pair")
	}

	keyPair := KeyPair{
		Id:   &FeedRef{ID: kp.Public[:], Algo: "ed25519"},
		Pair: *kp,
	}
	return &keyPair, nil
}

func SaveKeyPair(kp *KeyPair, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "ssb.SaveKeyPair: failed to create file")
	}
	if kp.Id.Algo != "ed25519" {
		return errors.Wrapf(err, "ssb.SaveKeyPair: unsupported key algo:%s", kp.Id.Algo)
	}
	var sec = ssbSecret{
		Curve:   "ed25519",
		ID:      kp.Id,
		Private: base64.StdEncoding.EncodeToString(kp.Pair.Secret[:]) + ".ed25519",
		Public:  base64.StdEncoding.EncodeToString(kp.Pair.Public[:]) + ".ed25519",
	}
	if err := json.NewEncoder(f).Encode(sec); err != nil {
		return errors.Wrap(err, "ssb.SaveKeyPair: json encoding failed")
	}
	return errors.Wrap(f.Close(), "ssb.SaveKeyPair: failed to close file")
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
	var s ssbSecret
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, errors.Wrapf(err, "ssb.Parse: JSON decoding failed")
	}

	public, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(s.Public, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.Parse: base64 decode of public part failed")
	}

	private, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(s.Private, ".ed25519"))
	if err != nil {
		return nil, errors.Wrapf(err, "ssb.Parse: base64 decode of private part failed")
	}

	var kp secrethandshake.EdKeyPair
	copy(kp.Public[:], public)
	copy(kp.Secret[:], private)

	ssbkp := KeyPair{
		Id:   s.ID,
		Pair: kp,
	}
	return &ssbkp, nil
}
