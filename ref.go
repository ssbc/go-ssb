package ssb

import (
	"encoding/base64"
	"strings"

	"github.com/pkg/errors"
)

type Ref struct {
	Type RefType
	Data string
	Algo RefAlgo
}

type RefType int

func (rt RefType) String() string {
	switch rt {
	case RefFeed:
		return "@"
	case RefMessage:
		return "%"
	case RefBlob:
		return "&"
	default:
		return "?"
	}
}

const (
	RefInvalid RefType = iota
	RefFeed
	RefMessage
	RefBlob
)

type RefAlgo int

func (ra RefAlgo) String() string {
	switch ra {
	case RefAlgoSha256:
		return "sha256"
	case RefAlgoEd25519:
		return "ed25519"
	default:
		return "???"
	}
}

const (
	RefAlgoInvalid RefAlgo = iota
	RefAlgoSha256
	RefAlgoEd25519
)

var (
	ErrInvalidRefType = errors.New("Invalid Ref Type")
	ErrInvalidRefAlgo = errors.New("Invalid Ref Algo")
	ErrInvalidSig     = errors.New("Invalid Signature")
	ErrInvalidHash    = errors.New("Invalid Hash")
)

func NewRef(typ RefType, raw []byte, algo RefAlgo) (Ref, error) {
	return Ref{typ, string(raw), algo}, nil
}

func (r Ref) Raw() []byte {
	return []byte(r.Data)
}

func ParseRef(ref string) (Ref, error) {
	parts := strings.Split(strings.Trim(ref, "@%&"), ".")
	if len(parts) != 2 {
		return Ref{}, errors.Errorf("ssb ref: malformed type")
	}
	r := Ref{}
	switch ref[0] {
	case '@':
		r.Type = RefFeed
	case '%':
		r.Type = RefMessage
	case '&':
		r.Type = RefBlob
	default:
		return Ref{}, errors.Errorf("ssb ref: unknown type")
	}
	switch strings.ToLower(parts[1]) {
	case "sha256":
		r.Algo = RefAlgoSha256
	case "ed25519":
		r.Algo = RefAlgoEd25519
	default:
		return Ref{}, errors.Errorf("ssb ref: unknown algo")
	}
	buf, err := base64.StdEncoding.DecodeString(parts[0])
	r.Data = string(buf)
	return r, errors.Wrap(err, "decode failed")
}

func (r Ref) String() string {
	if r.Type == RefInvalid || r.Algo == RefAlgoInvalid {
		return ""
	}
	return r.Type.String() + base64.StdEncoding.EncodeToString([]byte(r.Data)) + "." + r.Algo.String()
}
func (r Ref) MarshalText() (text []byte, err error) {
	return []byte(r.String()), nil
}

func (r *Ref) UnmarshalText(text []byte) (err error) {
	*r, err = ParseRef(string(text))
	return
}
