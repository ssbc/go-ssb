package ssb

import (
	"encoding/json"
)

type SignedMessage struct {
	Message
	Signature Signature `json:"signature"`
}

type Message struct {
	Previous  *Ref            `json:"previous"`
	Author    Ref             `json:"author"`
	Sequence  int             `json:"sequence"`
	Timestamp float64         `json:"timestamp"`
	Hash      string          `json:"hash"`
	Content   json.RawMessage `json:"content"`
}
