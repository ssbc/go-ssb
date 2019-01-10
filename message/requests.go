package message

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/auth"
)

type WhoamiReply struct {
	ID *ssb.FeedRef `json:"id"`
}

func NewCreateHistArgsFromMap(argMap map[string]interface{}) (*CreateHistArgs, error) {

	// could reflect over qrys fiields but meh - compiler knows better
	var qry CreateHistArgs
	for k, v := range argMap {
		switch k = strings.ToLower(k); k {
		case "live", "keys", "values", "reverse":
			b, ok := v.(bool)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a bool for %s", k)
			}
			switch k {
			case "live":
				qry.Live = b
			case "keys":
				qry.Keys = b
			case "values":
				qry.Values = b
			case "reverse":
				qry.Reverse = b
			}

		case "type":
			fallthrough
		case "id":
			val, ok := v.(string)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a string for %s", k)
			}
			switch k {
			case "id":
				qry.Id = val
			case "type":
				qry.Type = val
			}
		case "seq", "limit":
			n, ok := v.(float64)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a float64(%T) for %s", v, k)
			}
			switch k {
			case "seq":
				qry.Seq = int64(n)
			case "limit":
				qry.Limit = int64(n)
			}
		}
	}

	if qry.Limit == 0 {
		qry.Limit = -1
	}

	return &qry, nil
}

type CreateHistArgs struct {
	Keys    bool   `json:"keys"`
	Values  bool   `json:"values"`
	Live    bool   `json:"live"`
	Id      string `json:"id"`
	Seq     int64  `json:"seq"`
	Limit   int64  `json:"limit"`
	Reverse bool   `json:"reverse"`
	Type    string `json:"type"`
}

type RawSignedMessage struct {
	json.RawMessage
}

type StoredMessage struct {
	Author    *ssb.FeedRef    // @... pubkey
	Previous  *ssb.MessageRef // %... message hashsha
	Key       *ssb.MessageRef // %... message hashsha
	Sequence  margaret.BaseSeq
	Timestamp time.Time
	Raw       []byte // the original message for gossiping see ssb.EncodePreserveOrdering for why
}

func (sm StoredMessage) String() string {
	return fmt.Sprintf("msg(%s) %s", sm.Author.Ref(), sm.Key.Ref())
}

type DeserializedMessage struct {
	Previous  ssb.MessageRef   `json:"previous"`
	Author    ssb.FeedRef      `json:"author"`
	Sequence  margaret.BaseSeq `json:"sequence"`
	Timestamp float64          `json:"timestamp"`
	Hash      string           `json:"hash"`
	Content   json.RawMessage  `json:"content"`
}

type LegacyMessage struct {
	Previous  *ssb.MessageRef  `json:"previous"`
	Author    string           `json:"author"`
	Sequence  margaret.BaseSeq `json:"sequence"`
	Timestamp int64            `json:"timestamp"`
	Hash      string           `json:"hash"`
	Content   interface{}      `json:"content"`
}

// Sign preserves the filed order (up to content)
func (msg LegacyMessage) Sign(priv ed25519.PrivateKey, hmacSecret *[32]byte) (*ssb.MessageRef, []byte, error) {
	// flatten interface{} content value
	pp, err := jsonAndPreserve(msg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "legacySign: error during sign prepare")
	}

	if hmacSecret != nil {
		mac := auth.Sum(pp, hmacSecret)
		pp = mac[:]
	}

	sig := ed25519.Sign(priv, pp)

	var signedMsg SignedLegacyMessage
	signedMsg.LegacyMessage = msg
	signedMsg.Signature = EncodeSignature(sig)

	// encode again, now with the signature to get the hash of the message
	ppWithSig, err := jsonAndPreserve(signedMsg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "legacySign: error re-encoding signed message")
	}

	v8warp, err := InternalV8Binary(ppWithSig)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "legacySign: could not v8 escape message")
	}

	h := sha256.New()
	io.Copy(h, bytes.NewReader(v8warp))

	mr := &ssb.MessageRef{
		Hash: h.Sum(nil),
		Algo: ssb.RefAlgoSHA256,
	}
	return mr, ppWithSig, nil
}

func jsonAndPreserve(msg interface{}) ([]byte, error) {
	var buf bytes.Buffer // might want to pass a bufpool around here
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return nil, errors.Wrap(err, "jsonAndPreserve: 1st-pass json flattning failed")
	}

	// pretty-print v8-like
	pp, err := EncodePreserveOrder(buf.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "jsonAndPreserve: preserver order failed")
	}
	return pp, nil
}

type SignedLegacyMessage struct {
	LegacyMessage
	Signature Signature `json:"signature"`
}

type KeyValueRaw struct {
	Key       *ssb.MessageRef `json:"key"`
	Value     json.RawMessage `json:"value"`
	Timestamp int64           `json:"timestamp"`
}

type Typed struct {
	Previous  ssb.MessageRef   `json:"previous"`
	Author    ssb.FeedRef      `json:"author"`
	Sequence  margaret.BaseSeq `json:"sequence"`
	Timestamp float64          `json:"timestamp"`
	Hash      string           `json:"hash"`
	Content   struct {
		Type string `json:"type"`
	} `json:"content"`
}

type KeyValueAsMap struct {
	Key   *ssb.MessageRef `json:"key"`
	Value struct {
		Previous  ssb.MessageRef   `json:"previous"`
		Author    ssb.FeedRef      `json:"author"`
		Sequence  margaret.BaseSeq `json:"sequence"`
		Timestamp float64          `json:"timestamp"`
		Hash      string           `json:"hash"`
		Content   interface{}      `json:"content"`
	} `json:"value"`
	Timestamp int64 `json:"timestamp"`
}
