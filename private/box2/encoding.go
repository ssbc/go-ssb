package box2

import (
	"fmt"

	"encoding/binary"

	refs "go.mindeco.de/ssb-refs"
)

// encodeUint16 encodes a uint16 with little-endian encoding,
// appends it to out returns the result.
func encodeUint16(out []byte, l uint16) []byte {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], l)
	return append(out, buf[:]...)
}

// encodeList appends the SLP-encoding of a list to out
// and returns the resulting slice.
func encodeList(out []byte, list [][]byte) []byte {
	for _, elem := range list {
		out = encodeUint16(out, uint16(len(elem)))
		out = append(out, elem...)
	}

	return out
}

const (
	// type are the type-format-key type values
	typeFeed = iota
	typeMessage
	typeBlob
)

const (
	// type are the type-format-key feed format values
	feedFormatEd25519 = iota
	feedFormatGabbyGrove
)
const (
	// type are the type-format-key message format values
	messageFormatSHA256 = iota
	messageFormatGabbyGrove
)

// encodeFeedRef appends the TFK-encoding of a feed reference
// to out and returns the resulting slice.
func encodeFeedRef(out []byte, ref *refs.FeedRef) []byte {
	switch ref.Algo {
	case "ed25519":
		out = append(out, typeFeed, feedFormatEd25519)
	case "gabby":
		out = append(out, typeFeed, feedFormatGabbyGrove)
	default:
		return nil
	}

	return append(out, ref.ID...)
}

// encodeMessageRef appends the TFK-encoding of a message reference
// to out and returns the resulting slice.
func encodeMessageRef(out []byte, ref *refs.MessageRef) []byte {
	switch ref.Algo {
	case "sha256":
		out = append(out, typeMessage, messageFormatSHA256)
	case "gabby":
		out = append(out, typeMessage, messageFormatGabbyGrove)
	default:
		return nil
	}

	return append(out, ref.Hash...)
}

// TODO: move tfk to refs
// TODO: implement TFK as binary.(Un)Marshaler

func feedRefFromTFK(input []byte) *refs.FeedRef {
	if len(input) < 2 {
		panic("broken TFK")
	}

	if input[0] != typeFeed {
		panic("not a feed")
	}

	ref := refs.FeedRef{}

	switch input[1] {
	case feedFormatEd25519:
		ref.Algo = "ed25519"
	case feedFormatGabbyGrove:
		ref.Algo = "ggfeed-v1"
	default:
		panic(fmt.Sprintf("feedFK: not a known format:%x %d", input, feedFormatEd25519))
	}

	ref.ID = make([]byte, 32)
	copy(ref.ID, input[2:])

	return &ref
}

func messageRefFromTFK(input []byte) *refs.MessageRef {
	if len(input) < 2 {
		panic("broken TFK")
	}

	if input[0] != typeMessage {
		panic("not a msg")
	}

	ref := refs.MessageRef{}

	switch input[1] {
	case messageFormatSHA256:
		ref.Algo = "sha256"
	case messageFormatGabbyGrove:
		ref.Algo = "ggmsg-v1"
	default:
		panic(fmt.Sprintf("msgTFK: not a known format:%x", input[1]))
	}

	ref.Hash = make([]byte, 32)
	copy(ref.Hash, input[2:])

	return &ref
}
