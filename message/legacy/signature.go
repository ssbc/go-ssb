// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	refs "github.com/ssbc/go-ssb-refs"
)

var (
	jsonLineSeparator   = []byte("\n")
	jsonSignatureSuffix = []byte(`.sig.ed25519"`)
	jsonQuote           = []byte(`"`)
	jsonComma           = []byte(`,`)
)

// ExtractSignature expects a pretty printed message and uses a regexp to strip it from the msg for signature verification
func ExtractSignature(b []byte) ([]byte, Signature, error) {
	// BUG(cryptix): this expects signature on the root of the object.
	// some functions (like createHistoryStream with keys:true) nest the message on level deeper and this fails
	// NOTE(boreq): this seems like expected behaviour to me.

	lines := bytes.Split(b, jsonLineSeparator)
	for i := len(lines) - 1; i >= 0; i-- {
		if bytes.HasSuffix(lines[i], jsonSignatureSuffix) {
			signature, err := extractSignature(lines[i])
			if err != nil {
				return nil, nil, fmt.Errorf("error extracting signature: %w", err)
			}

			lines = append(lines[:i], lines[i+1:]...)
			if i == 0 {
				return nil, nil, errors.New("this message seems malformed, signature should be at the end of it")
			}

			if !bytes.HasSuffix(lines[i-1], jsonComma) {
				return nil, nil, errors.New("this message seems malformed, previous line should have a comma at the end")
			}

			lines[i-1] = bytes.TrimSuffix(lines[i-1], jsonComma)
			msg := bytes.Join(lines, []byte("\n"))
			return msg, signature, nil
		}
	}

	return nil, nil, errors.New("signature not found")
}

func extractSignature(line []byte) (Signature, error) {
	closingQuoteIndex := bytes.LastIndex(line, jsonQuote)
	if closingQuoteIndex < 0 {
		return nil, errors.New("closing quote not found")
	}
	line = line[:closingQuoteIndex]

	openingQuoteIndex := bytes.LastIndex(line, jsonQuote)
	if openingQuoteIndex < 0 {
		return nil, errors.New("opening quote not found")
	}

	line = line[openingQuoteIndex+1:]

	sig, err := NewSignatureFromBase64(line)
	if err != nil {
		return nil, fmt.Errorf("error creating signature from base64 data: %w", err)
	}

	return sig, nil
}

type Signature []byte

func NewSignatureFromBase64(input []byte) (Signature, error) {
	// check for and split off the suffix
	if !bytes.HasSuffix(input, signatureSuffix) {
		return nil, errors.New("ssb/signature: unexpected suffix")
	}
	b64 := bytes.TrimSuffix(input, signatureSuffix)

	// initial check of signature data to make sure it's within reasonable limits, to be checked in detail later due to padding issues
	// this is mainly to avoid decoding a signature that's obviously invalid and huge and filling up RAM in the process
	gotLen := base64.StdEncoding.DecodedLen(len(b64))
	if gotLen < ed25519.SignatureSize {
		return nil, fmt.Errorf("ssb/signature: expected more signature data but only got %d", gotLen)
	}
	if gotLen > ed25519.SignatureSize+2 {
		return nil, fmt.Errorf("ssb/signature: expected less signature data but got a string that could decode to up to %d bytes", gotLen)
	}

	// decode and check lengths
	decoded := make([]byte, gotLen)
	n, err := base64.StdEncoding.Decode(decoded, b64)
	if err != nil {
		return nil, fmt.Errorf("ssb/signature: invalid base64 data: %w", err)
	}
	decoded = decoded[:n]

	decodedLen := len(decoded)
	if decodedLen != ed25519.SignatureSize {
		return nil, fmt.Errorf("ssb/signature: decoded data is %d bytes long and should be %d", decodedLen, ed25519.SignatureSize)
	}

	return decoded, err
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
		return fmt.Errorf("invalid feed algorithm")
	}

	if ed25519.Verify(r.PubKey(), content, s) {
		return nil
	}

	return fmt.Errorf("invalid signature")
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
	signedMsg.Signature = sig

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
