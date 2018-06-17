package message

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"

	"github.com/pkg/errors"
	"go.cryptoscope.co/sbot"
)

// ExtractSignature expects a pretty printed message and uses a regexp to strip it from the msg for signature verification
func ExtractSignature(b []byte) ([]byte, Signature, error) {
	// BUG(cryptix): this expects signature on the root of the object.
	// some functions (like createHistoryStream with keys:true) nest the message on level deeper and this fails
	matches := signatureRegexp.FindSubmatch(b)
	if n := len(matches); n != 2 {
		return nil, "", errors.Errorf("message Encode: expected signature in formatted bytes. Only %d matches", n)
	}
	sig := Signature(matches[1])
	out := signatureRegexp.ReplaceAll(b, []byte{})
	return out, sig, nil
}

func Verify(raw []byte) (*sbot.MessageRef, *DeserializedMessage, error) {
	enc, err := EncodePreserveOrder(raw)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "ssb Verify: could not encode message: %q...", raw[:15])
	}

	// destroys it for the network layer but makes it easier to access its values
	var dmsg DeserializedMessage
	if err := json.Unmarshal(enc, &dmsg); err != nil {
		return nil, nil, errors.Wrapf(err, "ssb Verify: could not json.Unmarshal message: %q...", raw[:15])
	}

	woSig, sig, err := ExtractSignature(enc)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "ssb Verify(%s:%d): could not extract signature", dmsg.Author.Ref(), dmsg.Sequence)
	}

	if err := sig.Verify(woSig, &dmsg.Author); err != nil {
		return nil, nil, errors.Wrapf(err, "ssb Verify(%s:%d): could not verify message", dmsg.Author.Ref(), dmsg.Sequence)
	}

	// hash the message - it's sadly the internal string rep of v8 that get's hashed, not the json string
	v8warp, err := internalV8Binary(enc)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "ssb Verify(%s:%d): could hash convert message", dmsg.Author.Ref(), dmsg.Sequence)
	}
	h := sha256.New()
	io.Copy(h, bytes.NewReader(v8warp))

	mr := sbot.MessageRef{
		Hash: h.Sum(nil),
		Algo: sbot.RefAlgoSHA256,
	}
	return &mr, &dmsg, nil
}
