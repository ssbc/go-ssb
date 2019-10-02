package keys

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/pkg/errors"
	"golang.org/x/crypto/hkdf"
)

type Info []byte

func (info Info) Len() int {
	return 2 + len(info)
}

type Infos []Info

func (is Infos) Len() int {
	var l int

	for _, info := range is {
		l += info.Len()
	}

	return l
}

func (is Infos) Encode(out []byte) int {
	var used int

	for _, info := range is {
		binary.LittleEndian.PutUint16(out[used:], uint16(len(info)))
		used += 2
		used += copy(out[used:], info)
	}

	return used
}

// Recipient combines key data with a scheme
type Recipient struct {
	Key Key

	Scheme KeyScheme // (shared secret, diffie, etc)
}

// Recipients are a list of Recipients
type Recipients []Recipient

// Key holds actual key material
type Key []byte

// Derive returns a new key derived from the internal key data and the passed infos.
func (k Key) Derive(buf []byte, infos Infos, outLen int) (Key, error) {
	// if buffer is too short to hold everything, allocate
	if needed := infos.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	// first the out key because the rest can be freely reused by the caller
	_, out, buf := alloc(buf, outLen)

	// encode info and slice out the written bytes
	l := infos.Encode(buf)
	_, infoBs, buf := alloc(buf, l)

	// initialize and perform key derivation
	r := hkdf.New(sha256.New, []byte(k), nil, infoBs)
	_, err := r.Read(out)
	if err != nil {
		return nil, errors.Wrap(err, "error deriving key")
	}

	// clear intermediate data
	for i := range infoBs {
		infoBs[i] = 0
	}

	return Key(out), err
}

// Keys is a list of keys
type Keys []Key

func alloc(bs []byte, n int) (old, allocd, new []byte) {
	old, allocd, new = bs, bs[:n], bs[n:]
	return
}
