package box2

import (
	"encoding/binary"

	"go.cryptoscope.co/ssb"
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

	// type are the type-format-key feed format values
	feedFormatEd25519 = iota
	feedFormatGabbyGrove

	// type are the type-format-key message format values
	messageFormatSHA256 = iota
	messageFormatGabbyGrove
)

// encodeFeedRef appends the TFK-encoding of a feed reference
// to out and returns the resulting slice.
func encodeFeedRef(out []byte, ref *ssb.FeedRef) []byte {
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
func encodeMessageRef(out []byte, ref *ssb.MessageRef) []byte {
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
