package message

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

type WhoamiReply struct {
	ID string `json:"id"`
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

type SignedLegacyMessage struct {
	LegacyMessage
	Signature Signature `json:"signature"`
}
