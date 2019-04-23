package ssb

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"

	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
)

const (
	RefAlgoSHA256  = "sha256"
	RefAlgoEd25519 = "ed25519"
)

var (
	ErrInvalidRef     = errors.New("Invalid Ref")
	ErrInvalidRefType = errors.New("Invalid Ref Type")
	ErrInvalidRefAlgo = errors.New("Invalid Ref Algo")
	ErrInvalidSig     = errors.New("Invalid Signature")
	ErrInvalidHash    = errors.New("Invalid Hash")
)

type ErrRefLen struct {
	algo string
	n    int
}

func (e ErrRefLen) Error() string {
	return fmt.Sprintf("ssb: Invalid reference len for %s: %d", e.algo, e.n)
}

func NewFeedRefLenError(n int) error {
	return ErrRefLen{algo: RefAlgoEd25519, n: n}
}

func NewHashLenError(n int) error {
	return ErrRefLen{algo: RefAlgoSHA256, n: n}
}

func ParseRef(str string) (Ref, error) {
	if len(str) == 0 {
		return nil, ErrInvalidRef
	}

	split := strings.Split(str[1:], ".")
	if len(split) != 2 {
		return nil, ErrInvalidRef
	}

	raw, err := base64.StdEncoding.DecodeString(split[0])
	if err != nil { // ???
		raw, err = base64.URLEncoding.DecodeString(split[1])
		if err != nil {
			return nil, ErrInvalidHash
		}
	}

	switch str[0:1] {
	case "@":
		if split[1] != "ed25519" {
			return nil, ErrInvalidRefAlgo
		}
		if n := len(raw); n != 32 {
			return nil, NewFeedRefLenError(n)
		}
		return &FeedRef{
			ID:   raw,
			Algo: RefAlgoEd25519,
		}, nil
	case "%":
		if split[1] != "sha256" {
			return nil, ErrInvalidRefAlgo
		}
		if n := len(raw); n != 32 {
			return nil, NewHashLenError(n)
		}
		return &MessageRef{
			Hash: raw,
			Algo: RefAlgoSHA256,
		}, nil
	case "&":
		if split[1] != "sha256" {
			return nil, ErrInvalidRefAlgo
		}
		if n := len(raw); n != 32 {
			return nil, NewHashLenError(n)
		}
		return &BlobRef{
			Hash: raw,
			Algo: RefAlgoSHA256,
		}, nil
	}

	return nil, ErrInvalidRefType
}

type Ref interface {
	Ref() string
}

type BlobRef struct {
	Hash []byte
	Algo string
}

func (ref BlobRef) Ref() string {
	return fmt.Sprintf("&%s.%s", base64.StdEncoding.EncodeToString(ref.Hash), ref.Algo)
}

type MessageRef struct {
	Hash []byte
	Algo string
}

func (ref MessageRef) Ref() string {
	return fmt.Sprintf("%%%s.%s", base64.StdEncoding.EncodeToString(ref.Hash), ref.Algo)
}

type FeedRef struct {
	ID   []byte
	Algo string
}

func NewFeedRefEd25519(b [32]byte) (*FeedRef, error) {
	var r FeedRef
	r.Algo = RefAlgoEd25519
	if len(b) != 32 {
		return nil, ErrInvalidRef
	}
	r.ID = make([]byte, 32)
	copy(r.ID, b[:])
	return &r, nil
}

func (ref FeedRef) Ref() string {
	return fmt.Sprintf("@%s.%s", base64.StdEncoding.EncodeToString(ref.ID), ref.Algo)
}

var (
	_ encoding.TextMarshaler   = (*FeedRef)(nil)
	_ encoding.TextUnmarshaler = (*FeedRef)(nil)
)

func (fr *FeedRef) MarshalText() ([]byte, error) {
	return []byte(fr.Ref()), nil
}

func (fr *FeedRef) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*fr = FeedRef{}
		return nil
	}
	newRef, err := ParseFeedRef(string(text))
	*fr = *newRef
	return err
}

func (r *FeedRef) Scan(raw interface{}) error {
	switch v := raw.(type) {
	case []byte:
		if len(v) != 32 {
			return errors.Errorf("feedRef/Scan: wrong length: %d", len(v))
		}
		(*r).ID = v
		(*r).Algo = "ed25519"

	case string:
		fr, err := ParseFeedRef(v)
		if err != nil {
			return errors.Wrap(err, "feedRef/Scan: failed to serialze from string")
		}
		*r = *fr
	default:
		return errors.Errorf("feedRef/Scan: unhandled type %T", raw)
	}
	return nil
}

func ParseFeedRef(s string) (*FeedRef, error) {
	ref, err := ParseRef(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ref")
	}
	newRef, ok := ref.(*FeedRef)
	if !ok {
		return nil, errors.Errorf("feedRef: not a feed! %T", ref)
	}
	return newRef, nil
}

var (
	_ encoding.TextMarshaler   = (*MessageRef)(nil)
	_ encoding.TextUnmarshaler = (*MessageRef)(nil)
)

func (mr *MessageRef) MarshalText() ([]byte, error) {
	if len(mr.Hash) == 0 {
		return []byte{}, nil
	}
	return []byte(mr.Ref()), nil
}

func (mr *MessageRef) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*mr = MessageRef{}
		return nil
	}
	newRef, err := ParseMessageRef(string(text))
	if err != nil {
		return errors.Wrap(err, "message: unmarshal failed")
	}
	*mr = *newRef
	return nil
}

func (r *MessageRef) Scan(raw interface{}) error {
	switch v := raw.(type) {
	case []byte:
		if len(v) != 32 {
			return errors.Errorf("msgRef/Scan: wrong length: %d", len(v))
		}
		r.Hash = v
		r.Algo = "sha256"
	case string:
		mr, err := ParseMessageRef(v)
		if err != nil {
			return errors.Wrap(err, "msgRef/Scan: failed to serialze from string")
		}
		*r = *mr
	default:
		return errors.Errorf("msgRef/Scan: unhandled type %T", raw)
	}
	return nil
}

func ParseMessageRef(s string) (*MessageRef, error) {
	ref, err := ParseRef(s)
	if err != nil {
		return nil, errors.Wrap(err, "messageRef: failed to parse ref")
	}
	newRef, ok := ref.(*MessageRef)
	if !ok {
		return nil, errors.Errorf("messageRef: not a message! %T", ref)
	}
	return newRef, nil
}

func ParseBlobRef(s string) (*BlobRef, error) {
	ref, err := ParseRef(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ref")
	}
	newRef, ok := ref.(*BlobRef)
	if !ok {
		return nil, errors.Errorf("blobRef: not a blob! %T", ref)
	}
	return newRef, nil
}

func (br *BlobRef) MarshalText() ([]byte, error) {
	return []byte(br.Ref()), nil
}

func (br *BlobRef) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*br = BlobRef{}
		return nil
	}
	ref, err := ParseRef(string(text))
	if err != nil {
		return errors.Wrap(err, "failed to parse ref")
	}
	newRef, ok := ref.(*BlobRef)
	if !ok {
		return errors.Errorf("blobRef: not a blob! %T", ref)
	}
	*br = *newRef
	return nil
}

func GetFeedRefFromAddr(addr net.Addr) (*FeedRef, error) {
	addr = netwrap.GetAddr(addr, secretstream.NetworkString)
	if addr == nil {
		return nil, errors.New("no shs-bs address found")
	}

	ssAddr := addr.(secretstream.Addr)
	ref, err := ParseRef(ssAddr.String())
	if err != nil {
		return nil, errors.Wrap(err, "ref parse error")
	}

	fr, ok := ref.(*FeedRef)
	if !ok {
		return nil, fmt.Errorf("expected a %T but got a %v", fr, ref)
	}

	return fr, nil
}
