// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"
)

var signatureRegexp = regexp.MustCompile(",\n  \"signature\": \"([A-Za-z0-9/+=.]+)\"")

// ExtractSignature expects a pretty printed message and uses a regexp to strip it from the msg for signature verification
func ExtractSignature(b []byte) ([]byte, Signature, error) {
	// BUG(cryptix): this expects signature on the root of the object.
	// some functions (like createHistoryStream with keys:true) nest the message on level deeper and this fails
	matches := signatureRegexp.FindSubmatch(b)
	if n := len(matches); n != 2 {
		return nil, nil, fmt.Errorf("ssb/ExtractSignature: expected signature in formatted bytes. Only %d matches", n)
	}
	msg := signatureRegexp.ReplaceAll(b, []byte{})

	sig, err := NewSignatureFromBase64(matches[1])
	if err != nil {
		return nil, nil, fmt.Errorf("ssb/ExtractSignature: invalid base64 data: %w", err)
	}
	return msg, sig, nil
}

type Signature []byte

func NewSignatureFromBase64(input []byte) (Signature, error) {
	// check for and split off the suffix
	if !bytes.HasSuffix(input, signatureSuffix) {
		return nil, errors.New("ssb/signature: unexpected suffix")
	}
	b64 := bytes.TrimSuffix(input, signatureSuffix)

	// check we have at least 64 bytes of signature data
	dataLen := len(b64)
	gotLen := base64.StdEncoding.DecodedLen(dataLen)
	if gotLen < ed25519.SignatureSize {
		return nil, fmt.Errorf("ssb/signature: expected more signature data but only got %d", gotLen)
	}

	// allocate space for the signature and copy data into it
	decoded := make([]byte, gotLen)
	n, err := base64.StdEncoding.Decode(decoded, b64)
	if err != nil {
		return nil, fmt.Errorf("ssb/signature: invalid base64 data: %w", err)
	}

	if n > ed25519.SignatureSize {
		return nil, fmt.Errorf("ssb/signature: too much signature data (%d)", n)
	}

	decoded = decoded[:ed25519.SignatureSize]

	return Signature(decoded), err
}

var signatureSuffix = []byte(".sig.ed25519")

// UnmarshalJSON turns a std base64 encoded string with the suffix into JSON data
func (s *Signature) UnmarshalJSON(input []byte) error {
	if !(input[0] == '"' && input[len(input)-1] == '"') {
		return errors.New("ssb/signature: not a string")
	}

	// strip of the string markers
	input = input[1 : len(input)-1]

	newSig, err := NewSignatureFromBase64(input)
	if err != nil {
		return err
	}

	*s = newSig
	return nil
}

// MarshalJSON turns the binary signature data into a base64 string with the SSB suffix
func (s Signature) MarshalJSON() ([]byte, error) {
	// 2 for the string markers
	dataLen := base64.StdEncoding.EncodedLen(len(s))
	totalLen := 2 + dataLen + len(signatureSuffix)

	enc := make([]byte, totalLen)

	// make it a string
	enc[0] = '"'
	enc[totalLen-1] = '"'

	// copy in the base64 data
	base64.StdEncoding.Encode(enc[1:1+dataLen], s)

	// copy the suffix to the end
	copy(enc[1+dataLen:], signatureSuffix)

	return enc, nil
}

func (s Signature) Verify(content []byte, r refs.FeedRef) error {
	algo := r.Algo()
	if algo != refs.RefAlgoFeedSSB1 && algo != refs.RefAlgoFeedBendyButt {
		return errors.New("invalid feed algorithm")
	}

	if n := len(s); n != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size (%d bytes)", n)
	}

	if !ed25519.Verify(r.PubKey(), content, s) {
		return errors.New("invalid signature")
	}

	return nil
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
		return emptyMsgRef, nil, fmt.Errorf("legacySign: error during sign prepare: %w", err)
	}

	pp = maybeHMAC(pp, hmacSecret)

	sig := ed25519.Sign(priv, pp)

	var signedMsg SignedLegacyMessage
	signedMsg.LegacyMessage = msg
	signedMsg.Signature = sig

	// encode again, now with the signature to get the hash of the message
	ppWithSig, err := jsonAndPreserve(signedMsg)
	if err != nil {
		return emptyMsgRef, nil, fmt.Errorf("legacySign: error re-encoding signed message: %w", err)
	}

	v8warp, err := InternalV8Binary(ppWithSig)
	if err != nil {
		return emptyMsgRef, nil, fmt.Errorf("legacySign: could not v8 escape message: %w", err)
	}

	h := sha256.New()
	h.Write(v8warp)

	mr, err := refs.NewMessageRefFromBytes(h.Sum(nil), refs.RefAlgoMessageSSB1)
	if err != nil {
		return emptyMsgRef, nil, fmt.Errorf("legacySign: failed to construct new message hash: %w", err)
	}

	condensed, err := json.Marshal(signedMsg)
	if err != nil {
		return emptyMsgRef, nil, fmt.Errorf("legacySign: failed to marshal space-less message: %w", err)
	}

	return mr, condensed, nil
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
