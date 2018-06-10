package message

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"

	"cryptoscope.co/go/sbot"
	"github.com/pkg/errors"
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
		return nil, nil, errors.Wrap(err, "ssb: could not verify message")
	}

	woSig, sig, err := ExtractSignature(enc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "ssb: could not extract signature")
	}

	foundAuthor := authorRegexp.FindSubmatch(enc)
	if len(foundAuthor) != 2 {
		return nil, nil, errors.Errorf("ssb: did not find author. Matches:%d", len(foundAuthor))
	}

	ref, err := sbot.ParseRef(string(foundAuthor[1]))
	if err != nil {
		return nil, nil, errors.Wrap(err, "ssb: could not extract signature")
	}
	authorRef, ok := ref.(*sbot.FeedRef)
	if !ok {
		return nil, nil, errors.Errorf("ssb: unexpected ref type for author: %T", ref)
	}

	if err := sig.Verify(woSig, authorRef); err != nil {
		return nil, nil, errors.Wrap(err, "ssb: could not verify message")
	}

	v8warp, err := internalV8Binary(enc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "ssb: could hash convert message")
	}
	// hash the message
	h := sha256.New()
	io.Copy(h, bytes.NewReader(v8warp))

	// destroys it for the network protocl but makes it easier to access its values
	var dmsg DeserializedMessage
	if err := json.Unmarshal(enc, &dmsg); err != nil {
		return nil, nil, errors.Wrap(err, "ssb: could not verify message")
	}
	mr := sbot.MessageRef{
		Hash: h.Sum(nil),
		Algo: sbot.RefAlgoSHA256,
	}
	return &mr, &dmsg, nil
}
