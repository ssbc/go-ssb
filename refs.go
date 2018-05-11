package sbot

import (
	"encoding/base64"
	"fmt"
)

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
