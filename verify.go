package ssb

import (
	"bytes"
	"crypto/sha256"
	"io"

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

func Verify(raw []byte) (*Ref, error) {
	enc, err := EncodePreserveOrder(raw)
	if err != nil {
		return nil, errors.Wrap(err, "ssb: could not verify message")
	}

	woSig, sig, err := ExtractSignature(enc)
	if err != nil {
		return nil, errors.Wrap(err, "ssb: could not extract signature")
	}

	foundAuthor := authorRegexp.FindSubmatch(enc)
	if len(foundAuthor) != 2 {
		return nil, errors.Errorf("ssb: did not find author. Matches:%d", len(foundAuthor))
	}

	authorRef, err := ParseRef(string(foundAuthor[1]))
	if err != nil {
		return nil, errors.Wrap(err, "ssb: could not extract signature")
	}
	if authorRef.Type != RefFeed {
		return nil, errors.Errorf("ssb: unexpected ref type for author: %d", authorRef.Type)
	}

	if err := sig.Verify(woSig, authorRef); err != nil {
		return nil, errors.Wrap(err, "ssb: could not verify message")
	}

	// hash the message
	h := sha256.New()
	io.Copy(h, bytes.NewReader(enc))

	msgHash, err := NewRef(RefMessage, h.Sum(nil), RefAlgoSHA256)
	if err != nil {
		return nil, errors.Wrap(err, "ssb: could not extract signature")
	}

	return &msgHash, nil
}
