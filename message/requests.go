package message

import (
	"encoding/json"
	"time"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/sbot"
)

type WhoamiReply struct {
	ID string `json:"id"`
}

type CreateHistArgs struct {
	//map[keys:false id:@Bqm7bG4qvlnWh3BEBFSj2kDr+     30+mUU3hRgrikE2+xc=.ed25519 seq:20 live:true
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
	Author    *sbot.FeedRef    // @... pubkey
	Previous  *sbot.MessageRef // %... message hashsha
	Key       *sbot.MessageRef // %... message hashsha
	Sequence  margaret.BaseSeq
	Timestamp time.Time
	Raw       []byte // the original message for gossiping see ssb.EncodePreserveOrdering for why
}

type DeserializedMessage struct {
	Previous  sbot.MessageRef  `json:"previous"`
	Author    sbot.FeedRef     `json:"author"`
	Sequence  margaret.BaseSeq `json:"sequence"`
	Timestamp float64          `json:"timestamp"`
	Hash      string           `json:"hash"`
	Content   json.RawMessage  `json:"content"`
}
