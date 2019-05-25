package multimsg

import (
	"bytes"
	"encoding/gob"
	"log"

	"github.com/pkg/errors"
	"github.com/ugorji/go/codec"
	"go.mindeco.de/ssb-gabbygrove"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"
)

type MessageType byte

const (
	Unknown MessageType = iota
	Legacy
	Gabby
)

// MultiMessage attempts to support multiple message formats in the same storage layer
// currently supports Proto and legacy
type MultiMessage struct {
	ssb.Message
	tipe MessageType
	key  *ssb.MessageRef
}

func (mm MultiMessage) MarshalBinary() ([]byte, error) {
	switch mm.tipe {
	case Legacy:
		legacy, ok := mm.AsLegacy()
		if !ok {
			return nil, errors.Errorf("not a legacy message: %T", mm.Message)
		}
		var buf bytes.Buffer
		buf.Write([]byte{byte(Legacy)})

		var mh codec.MsgpackHandle
		enc := codec.NewEncoder(&buf, &mh)
		err := enc.Encode(legacy)
		if err != nil {
			return nil, errors.Wrap(err, "multiMessage: legacy encoding failed")
		}
		return buf.Bytes(), nil
	case Gabby:
		gabby, ok := mm.AsGabby()
		if !ok {
			return nil, errors.Errorf("multiMessage: wrong type of message: %T", mm.Message)
		}
		var buf bytes.Buffer
		buf.Write([]byte{byte(Gabby)})
		err := gob.NewEncoder(&buf).Encode(gabby)
		return buf.Bytes(), errors.Wrap(err, "multiMessage: gabby encoding failed")
	}
	return nil, errors.Errorf("multiMessage: unsupported message type: %x", mm.tipe)
}

func (mm *MultiMessage) UnmarshalBinary(data []byte) error {
	if len(data) < 1 {
		return errors.Errorf("multiMessage: data to short")
	}

	mm.tipe = MessageType(data[0])
	switch mm.tipe {
	case Legacy:
		var msg legacy.StoredMessage
		var mh codec.MsgpackHandle
		dec := codec.NewDecoderBytes(data[1:], &mh)
		err := dec.Decode(&msg)
		if err != nil {
			return errors.Wrap(err, "multiMessage: legacy decoding failed")
		}
		mm.Message = &msg
		mm.key = msg.Key_
	case Gabby:
		rd := bytes.NewReader(data[1:])
		var gb gabbygrove.Transfer
		err := gob.NewDecoder(rd).Decode(&gb)
		if err != nil {
			return errors.Wrap(err, "multiMessage: gabby decoding failed")
		}
		mm.Message = &gb
		mm.key = gb.Key()
	default:
		return errors.Errorf("multiMessage: unsupported message type: %x", mm.tipe)
	}
	return nil
}

func (mm MultiMessage) AsLegacy() (*legacy.StoredMessage, bool) {
	if mm.tipe != Legacy {
		return nil, false
	}
	legacy, ok := mm.Message.(*legacy.StoredMessage)
	if !ok {
		err := errors.Errorf("multiMessage: wrong type of message: %T", mm.Message)
		log.Println("AsLegacy", err)
		return nil, false
	}
	return legacy, true
}

func (mm MultiMessage) AsGabby() (*gabbygrove.Transfer, bool) {
	if mm.tipe != Gabby {
		return nil, false
	}
	gabby, ok := mm.Message.(*gabbygrove.Transfer)
	if !ok {
		err := errors.Errorf("multiMessage: wrong type of message: %T", mm.Message)
		log.Println("AsGabby", err)
		return nil, false
	}
	return gabby, true
}

func NewMultiMessageFromLegacy(msg *legacy.StoredMessage) *MultiMessage {
	var mm MultiMessage
	mm.tipe = Legacy
	mm.key = msg.Key_
	mm.Message = msg
	return &mm
}
