package box2

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/hkdf"
)

// encodeUint16 encodes a uint16 with little-endian encoding,
// appends it to out returns the result.
func encodeUint16(out []byte, l uint16) []byte {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], l)
	return append(out, buf[:]...)
}

// EncodeSLP appends the SLP-encoding of a list to out
// and returns the resulting slice.
func EncodeSLP(out []byte, list ...[]byte) []byte {
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

func DeriveTo(out, key []byte, infos ...[]byte) error {
	if n := len(out); n != 32 {
		return fmt.Errorf("box2: expected 32b as output argument, got %d", n)
	}
	slp := EncodeSLP(nil, infos...)
	if bytes.Equal(infos[0], []byte("cloaked_msg_id")) {
		fmt.Fprintln(os.Stderr, "[Go] read_key")
		spew.Dump(key)
		fmt.Fprintln(os.Stderr, "[Go] SLP")
		spew.Dump(slp)
	}
	r := hkdf.Expand(sha256.New, key, slp)
	nout, err := r.Read(out)
	if err != nil {
		return fmt.Errorf("box2: failed to derive key: %w", err)
	}

	if nout != 32 {
		return fmt.Errorf("box2: expected to read 32b into output, got %d", nout)
	}

	return nil
}
