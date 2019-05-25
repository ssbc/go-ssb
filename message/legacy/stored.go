package legacy

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

// really dislike the underlines..

type StoredMessage struct {
	Author_    *ssb.FeedRef    // @... pubkey
	Previous_  *ssb.MessageRef // %... message hashsha
	Key_       *ssb.MessageRef // %... message hashsha
	Sequence_  margaret.BaseSeq
	Timestamp_ time.Time
	Raw_       []byte // the original message for gossiping see ssb.EncodePreserveOrdering for why
}

// could use this to unexport fields, would require lots of constructors though
// func (sm StoredMessage) MarshalBinary() ([]byte, error) {
// }
// func (sm *StoredMessage) UnmarshalBinary(data []byte) error {
// }

func (sm StoredMessage) String() string {
	s := fmt.Sprintf("msg(%s:%d) %s", sm.Author_.Ref(), sm.Sequence_, sm.Key_.Ref())
	b, _ := EncodePreserveOrder(sm.Raw_)
	s += "\n"
	s += string(b)
	return s
}

var _ ssb.Message = (*StoredMessage)(nil)

func (sm StoredMessage) Seq() int64 {
	return sm.Sequence_.Seq()
}

func (sm StoredMessage) Key() *ssb.MessageRef {
	return sm.Key_
}

func (sm StoredMessage) Author() *ssb.FeedRef {
	return sm.Author_
}

func (sm StoredMessage) Previous() *ssb.MessageRef {
	return sm.Previous_
}

func (sm StoredMessage) Timestamp() time.Time {
	return sm.Timestamp_
}

func (sm StoredMessage) ContentBytes() []byte {
	var c struct {
		Content json.RawMessage `json:"content"`
	}
	err := json.Unmarshal(sm.Raw_, &c)
	if err != nil {
		log.Println("warning: Content of storedMessage failed:", err)
		return nil
	}
	return c.Content
}

func (sm StoredMessage) ValueContent() *ssb.Value {
	var msg ssb.Value
	msg.Previous = sm.Previous_
	msg.Author = *sm.Author_
	msg.Sequence = sm.Sequence_
	msg.Hash = "sha256"
	// msg.Timestamp = float64(sm.Timestamp.Unix() * 1000)
	var cs struct {
		Content   json.RawMessage `json:"content"`
		Signature string          `json:"signature"`
	}
	err := json.Unmarshal(sm.Raw_, &cs)
	if err != nil {
		log.Println("warning: Content of storedMessage failed:", err)
		return nil
	}
	msg.Content = cs.Content
	msg.Signature = cs.Signature
	return &msg
}

func (sm StoredMessage) ValueContentJSON() json.RawMessage {
	// jsonB, err := json.Marshal(sm.ValueContent())
	// if err != nil {
	// 	panic(err.Error())
	// }

	return sm.Raw_
}
