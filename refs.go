package sbot

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/pkg/errors"
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

func (ref *BlobRef) Ref() string {
	return fmt.Sprintf("&%s.%s", base64.StdEncoding.EncodeToString(ref.Hash), ref.Algo)
}

type MessageRef struct {
	Hash []byte
	Algo string
}

func (ref *MessageRef) Ref() string {
	return fmt.Sprintf("%%%s.%s", base64.StdEncoding.EncodeToString(ref.Hash), ref.Algo)
}

type FeedRef struct {
	ID   []byte
	Algo string
}

func (ref *FeedRef) Ref() string {
	return fmt.Sprintf("@%s.%s", base64.StdEncoding.EncodeToString(ref.ID), ref.Algo)
}

var (
	_ encoding.TextMarshaler   = (*FeedRef)(nil)
	_ encoding.TextUnmarshaler = (*FeedRef)(nil)
)

func (fr *FeedRef) MarshalText() (text []byte, err error) {
	return []byte(fr.Ref()), nil
}

func (fr *FeedRef) UnmarshalText(text []byte) error {
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

func (mr *MessageRef) MarshalText() (text []byte, err error) {
	return []byte(mr.Ref()), nil
}

func (mr *MessageRef) UnmarshalText(text []byte) error {
	ref, err := ParseRef(string(text))
	if err != nil {
		return errors.Wrap(err, "failed to parse ref")
	}

	newRef, ok := ref.(*MessageRef)
	if !ok {
		return errors.Errorf("feedRef: not a feed! %T", ref)
	}
	*mr = *newRef
	return nil
}
