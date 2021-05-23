// SPDX-License-Identifier: MIT

// Package legacy how to encode and verify the current ssb messages.
// You most likely want to use legacy.Verify() in most cases.
//
// See https://spec.scuttlebutt.nz/feed/messages.html for more encoding details.
package legacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/nacl/auth"
)

// ExtractSignature expects a pretty printed message and uses a regexp to strip it from the msg for signature verification
func ExtractSignature(b []byte) ([]byte, Signature, error) {
	// BUG(cryptix): this expects signature on the root of the object.
	// some functions (like createHistoryStream with keys:true) nest the message on level deeper and this fails
	matches := signatureRegexp.FindSubmatch(b)
	if n := len(matches); n != 2 {
		return nil, "", fmt.Errorf("message Encode: expected signature in formatted bytes. Only %d matches", n)
	}
	sig := Signature(matches[1])
	out := signatureRegexp.ReplaceAll(b, []byte{})
	return out, sig, nil
}

var (
	emptyMsgRef = refs.MessageRef{}
	emptyDMsg   = DeserializedMessage{}
)

// Verify takes an slice of bytes (like json.RawMessage) and uses EncodePreserveOrder to pretty print it.
// It then uses ExtractSignature and verifies the found signature against the author field of the message.
// If hmacSecret is non nil, it uses that as the Key for NACL crypto_auth() and verifies the signature against the hash of the message.
// At last it uses internalV8Binary to create a the SHA256 hash for the message key.
// If you find a buggy message, use `node ./encode_test.js $feedID` to generate a new testdata.zip
func Verify(raw []byte, hmacSecret *[32]byte) (refs.MessageRef, DeserializedMessage, error) {
	enc, err := EncodePreserveOrder(raw)
	if err != nil {
		if len(raw) > 15 {
			raw = raw[:15]
		}
		return emptyMsgRef, emptyDMsg, fmt.Errorf("ssb Verify: could not encode message (%q): %w", raw, err)
	}

	// destroys it for the network layer but makes it easier to access its values
	var dmsg DeserializedMessage
	if err := json.Unmarshal(raw, &dmsg); err != nil {
		if len(raw) > 15 {
			raw = raw[:15]
		}
		return emptyMsgRef, emptyDMsg, fmt.Errorf("ssb Verify: could not json.Unmarshal message (%q): %w", raw, err)
	}

	woSig, sig, err := ExtractSignature(enc)
	if err != nil {
		return emptyMsgRef, emptyDMsg, fmt.Errorf("ssb Verify(%s:%d): could not extract signature: %w", dmsg.Author.Ref(), dmsg.Sequence, err)
	}

	if hmacSecret != nil {
		mac := auth.Sum(woSig, hmacSecret)
		woSig = mac[:]
	}

	if err := sig.Verify(woSig, dmsg.Author); err != nil {
		return emptyMsgRef, emptyDMsg, fmt.Errorf("ssb Verify(%s:%d): %w", dmsg.Author.Ref(), dmsg.Sequence, err)
	}

	// hash the message - it's sadly the internal string rep of v8 that get's hashed, not the json string
	v8warp, err := InternalV8Binary(enc)
	if err != nil {
		return emptyMsgRef, emptyDMsg, fmt.Errorf("ssb Verify(%s:%d): could hash convert message: %w", dmsg.Author.Ref(), dmsg.Sequence, err)
	}
	h := sha256.New()
	io.Copy(h, bytes.NewReader(v8warp))

	mr, err := refs.NewMessageRefFromBytes(h.Sum(nil), refs.RefAlgoMessageSSB1)
	return mr, dmsg, err
}
