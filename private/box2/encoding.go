package box2

import (
	"crypto/sha256"
	"encoding/binary"

	"golang.org/x/crypto/hkdf"
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

/*
	Key Derivation scheme

	SharedSecret
	 |
	 +-> SlotKey

	MessageKey (randomly sampled by author)
	 |
	 +-> ReadKey
	 |    |
	 |    +-> HeaderKey
     |    |
     |    +-> BodyKey
	 |
	 +-> ExtensionsKey (TODO)
	      |
		  +-> (TODO: Ratcheting, ...)
*/

func deriveTo(out, key []byte, infos ...[]byte) {
	r := hkdf.Expand(sha256.New, key, encodeList(nil, infos))
	r.Read(out)
}
