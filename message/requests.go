package message

import (
	"encoding/json"
	"fmt"
	"time"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

type WhoamiReply struct {
	ID string `json:"id"`
}

type CreateHistArgs struct {
	Keys    bool   `json:"keys"`
	Values  bool   `json:"values"`
	Live    bool   `json:"live"`
	Id      string `json:"id"`
	Seq     int64  `json:"seq"`
	Limit   int64  `json:"limit"`
	Reverse bool   `json:"reverse"`
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

type KeyValueRaw struct {
	Key       *ssb.MessageRef `json:"key"`
	Value     json.RawMessage `json:"value"`
	Timestamp int64           `json:"timestamp"`
}
