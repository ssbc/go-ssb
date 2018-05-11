package ssb

import (
	"encoding/base64"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

type Signature string

type SigAlgo int

const (
	SigAlgoInvalid SigAlgo = iota
	SigAlgoEd25519
)

func (s Signature) Algo() SigAlgo {
	parts := strings.Split(string(s), ".")
	if len(parts) != 3 || parts[1] != "sig" {
		return SigAlgoInvalid
	}
	switch strings.ToLower(parts[2]) {
	case "ed25519":
		return SigAlgoEd25519
	}
	return SigAlgoInvalid
}

func (s Signature) Raw() ([]byte, error) {
	b64 := strings.Split(string(s), ".")[0]
	return base64.StdEncoding.DecodeString(b64)
}

func (s Signature) Verify(content []byte, r *Ref) error {
	switch s.Algo() {
	case SigAlgoEd25519:
		if r.Type != RefFeed {
			return ErrInvalidRefType
		}
		if r.Algo != RefAlgoEd25519 {
			return ErrInvalidSig
		}
		rawkey := r.Raw()
		if rawkey == nil {
			return nil
		}
		key := ed25519.PublicKey(rawkey)
		b, err := s.Raw()
		if err != nil {
			return errors.Wrap(err, "verify: raw unpack failed")
		}
		if ed25519.Verify(key, content, b) {
			return nil
		}
		return ErrInvalidSig
	default:
		return errors.Errorf("verify: unknown Algo")
	}
	return ErrInvalidSig
}
