// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"
)

type Signature string

func EncodeSignature(s []byte) Signature {
	return Signature(base64.StdEncoding.EncodeToString(s) + ".sig.ed25519")
}

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

func (s Signature) Bytes() ([]byte, error) {
	parts := strings.Split(string(s), ".")
	if n := len(parts); n < 1 {
		return nil, fmt.Errorf("signature: expected at least one part - got %d", n)
	}
	b64 := parts[0]
	return base64.StdEncoding.DecodeString(b64)
}

func (s Signature) Verify(content []byte, r refs.FeedRef) error {
	switch s.Algo() {
	case SigAlgoEd25519:
		algo := r.Algo()
		if algo != refs.RefAlgoFeedSSB1 && algo != refs.RefAlgoFeedBendyButt {
			return fmt.Errorf("invalid feed algorithm")
		}

		b, err := s.Bytes()
		if err != nil {
			return fmt.Errorf("unpack failed: %w", err)
		}

		if ed25519.Verify(r.PubKey(), content, b) {
			return nil
		}

		return fmt.Errorf("invalid signature")
	default:
		return fmt.Errorf("unknown signature algorithm")
	}
}

type LegacyMessage struct {
	Previous  *refs.MessageRef `json:"previous"`
	Author    string           `json:"author"`
	Sequence  int64            `json:"sequence"`
	Timestamp int64            `json:"timestamp"`
	Hash      string           `json:"hash"`
	Content   interface{}      `json:"content"`
}

type SignedLegacyMessage struct {
	LegacyMessage
	Signature Signature `json:"signature"`
}

// Sign preserves the filed order (up to content)
func (msg LegacyMessage) Sign(priv ed25519.PrivateKey, hmacSecret *[32]byte) (refs.MessageRef, []byte, error) {
	// flatten interface{} content value
	pp, err := jsonAndPreserve(msg)
	if err != nil {
		return refs.MessageRef{}, nil, fmt.Errorf("legacySign: error during sign prepare: %w", err)
	}

	pp = maybeHMAC(pp, hmacSecret)

	sig := ed25519.Sign(priv, pp)

	var signedMsg SignedLegacyMessage
	signedMsg.LegacyMessage = msg
	signedMsg.Signature = EncodeSignature(sig)

	// encode again, now with the signature to get the hash of the message
	ppWithSig, err := jsonAndPreserve(signedMsg)
	if err != nil {
		return refs.MessageRef{}, nil, fmt.Errorf("legacySign: error re-encoding signed message: %w", err)
	}

	v8warp, err := InternalV8Binary(ppWithSig)
	if err != nil {
		return refs.MessageRef{}, nil, fmt.Errorf("legacySign: could not v8 escape message: %w", err)
	}

	h := sha256.New()
	io.Copy(h, bytes.NewReader(v8warp))

	mr, err := refs.NewMessageRefFromBytes(h.Sum(nil), refs.RefAlgoMessageSSB1)
	return mr, ppWithSig, err
}

func jsonAndPreserve(msg interface{}) ([]byte, error) {
	var buf bytes.Buffer // might want to pass a bufpool around here
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return nil, fmt.Errorf("jsonAndPreserve: 1st-pass json flattning failed: %w", err)
	}

	// pretty-print v8-like
	pp, err := PrettyPrint(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("jsonAndPreserve: preserver order failed: %w", err)
	}
	return pp, nil
}
