package sbot

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

func ParseRef(str string) (Ref, error) {
	if len(str) == 0 {
		return nil, ErrInvalidRef
	}

	split := strings.Split(str[1:], ".")
	if len(split) != 2 {
		return nil, ErrInvalidRef
	}

	raw, err := base64.StdEncoding.DecodeString(split[0])
	if err != nil {
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

		return &FeedRef{
			ID:   raw,
			Algo: RefAlgoEd25519,
		}, nil
	case "%":
		if split[1] != "sha256" {
			return nil, ErrInvalidRefAlgo
		}

		return &MessageRef{
			Hash: raw,
			Algo: RefAlgoSHA256,
		}, nil
	case "&":
		if split[1] != "sha256" {
			return nil, ErrInvalidRefAlgo
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
	ref, err := ParseRef(string(text))
	if err != nil {
		return errors.Wrap(err, "failed to parse ref")
	}
	newRef, ok := ref.(*FeedRef)
	if !ok {
		return errors.Errorf("feedRef: not a feed! %T", ref)
	}
	*fr = *newRef
	return nil
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
	ref, err := ParseRef(string(text))
	if err != nil {
		return errors.Wrap(err, "failed to parse ref")
	}
	newRef, ok := ref.(*MessageRef)
	if !ok {
		return errors.Errorf("msgRef: not a message! %T", ref)
	}
	*mr = *newRef
	return nil
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
