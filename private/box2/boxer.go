package box2

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb/keys"
	"golang.org/x/crypto/nacl/secretbox"
)

type Message struct {
	Raw []byte

	HeaderBox   []byte
	AfterHeader []byte

	OffBody  int
	RawSlots []byte
	BodyBox  []byte
}

func NewBoxer(rand io.Reader) *Boxer {
	return &Boxer{rand: rand}
}

type Boxer struct {
	// TODO store base infos?
	// TODO use a simple buffer pool?
	rand io.Reader
}

const KeySize = 256 / 8

func (bxr *Boxer) Encrypt(buf, msg []byte, infos keys.Infos, ks keys.Keys) (*Message, error) {
	var outMsg Message

	// TODO Verify if this is indeed the right amount of memory
	if needed := 32 + len(ks)*32 + len(msg) + 16 + 4*32; len(buf) < needed {
		buf = make([]byte, needed)
	}

	var used int

	used += KeySize
	msgKey := MessageKey(buf[:KeySize])
	buf = buf[KeySize:]
	_, err := bxr.rand.Read([]byte(msgKey))
	if err != nil {
		return nil, errors.Wrap(err, "error reading random data")
	}

	// The keys returned by the Derive... functions are subslices of buf.
	// The calls to copy() in between are so we can safely reuse it.

	used += KeySize
	readKey := ReadKey(buf[:KeySize])
	buf = buf[KeySize:]
	readKey_, err := msgKey.DeriveReadKey(buf, infos, KeySize)
	if err != nil {
		return nil, errors.Wrap(err, "error deriving read key")
	}

	copy(readKey, readKey_)

	used += KeySize
	bodyKey := BodyKey(buf[:KeySize])
	buf = buf[KeySize:]
	bodyKey_, err := readKey.DeriveBodyKey(buf, infos, KeySize)
	if err != nil {
		return nil, errors.Wrap(err, "error deriving body key")
	}

	copy(bodyKey, bodyKey_)

	used += KeySize
	headerKey := HeaderKey(buf[:KeySize])
	buf = buf[KeySize:]
	headerKey_, err := readKey.DeriveHeaderKey(buf, infos, KeySize)
	if err != nil {
		return nil, errors.Wrap(err, "error deriving header key")
	}

	copy(headerKey, headerKey_)

	var k [32]byte

	// First append header, then slots, then body.
	// then we can return the entire thing as the message.

	// append header (todo)

	// header length + len(rceps) * slot length
	var bodyOff uint16 = 32 + uint16(len(ks))*32
	used += 16

	// header plaintext
	header := buf[:16]
	buf = buf[16:]
	binary.LittleEndian.PutUint16(header, bodyOff)

	// store this buffer as beginning of ciphertext
	ctxtLen := (len(ks)+1)*KeySize + len(msg) + secretbox.Overhead
	outMsg.Raw = buf[:ctxtLen]

	// header ciphertext
	outMsg.HeaderBox = buf[:0]
	used += 32
	buf = buf[32:]

	copy(k[:], []byte(headerKey))
	outMsg.HeaderBox = secretbox.Seal(outMsg.HeaderBox, header, &zero24, &k)

	// append slots
	for _, bk := range ks {
		mk, err := bk.Derive(buf, infos, KeySize)
		if err != nil {
			return nil, errors.Wrap(err, "error deriving recipient key")
		}

		slot := buf[:KeySize]

		for i := range mk {
			slot[i] = mk[i] ^ msgKey[i]
		}

		used += KeySize
		buf = buf[KeySize:]
	}

	// (append padding (deferred))

	// append body
	outMsg.BodyBox = buf[:0]
	copy(k[:], []byte(bodyKey))
	outMsg.BodyBox = secretbox.Seal(outMsg.BodyBox, msg, &zero24, &k)

	return &outMsg, nil
}

const MaxSlots = 32

var zero24 [24]byte
var zeroKey [KeySize]byte

// TODO: Maybe return entire decrypted message?
func (bxr *Boxer) Decrypt(buf, ctxt []byte, infos keys.Infos, ks keys.Keys) ([]byte, error) {
	if needed := len(ctxt) + len(ks)*KeySize; len(buf) < needed {
		buf = make([]byte, needed)
	}

	var msg Message

	msg.Raw = ctxt
	msg.HeaderBox = buf[:32]
	copy(msg.HeaderBox, ctxt[:32])
	msg.AfterHeader = buf[32:len(ctxt)]
	copy(msg.AfterHeader, ctxt[32:])

	buf = buf[32+len(ctxt):]

	dks := buf[:len(ks)*KeySize]
	buf = buf[len(ks)*KeySize:]

	for i, bk := range ks {
		mk, err := bk.Derive(buf, infos, KeySize)
		if err != nil {
			return nil, errors.Wrap(err, "error deriving recipient key")
		}

		dst := dks[i*KeySize : (i+1)*KeySize]
		copy(dst, []byte(mk))
	}

	var (
		undo, hdr       []byte
		msgKey, readKey []byte
		ok              bool
		key             [KeySize]byte
		i               int
	)

	defer copy(key[:], zeroKey[:])

	_, msgKey, buf = alloc(buf, KeySize)
	_, readKey, buf = alloc(buf, KeySize)

	undo, hdr, buf = alloc(buf, 16)

	for i = 0; i*KeySize < len(msg.AfterHeader) && i*KeySize < len(dks) && i < MaxSlots; i++ {
		for j := range dks[i*KeySize : (i+1)*KeySize] {
			msgKey[j] = dks[i*KeySize+j] ^ msg.AfterHeader[i*KeySize+j]
		}

		// we need the copies so we can safely reuse the buffer

		readKey_, err := MessageKey(msgKey).DeriveReadKey(buf, infos, KeySize)
		if err != nil {
			return nil, errors.Wrap(err, "error deriving read key")
		}
		copy(readKey, readKey_)

		hdrKey_, err := ReadKey(readKey).DeriveHeaderKey(buf, infos, KeySize)
		if err != nil {
			return nil, errors.Wrap(err, "error deriving header key")
		}
		copy(key[:], hdrKey_)

		hdr, ok = secretbox.Open(hdr[:0], msg.HeaderBox, &zero24, &key)
		if ok {
			break
		}
	}

	if !ok {
		return nil, fmt.Errorf("could not decrypt message")
	}

	msg.OffBody = int(binary.LittleEndian.Uint16(hdr))

	// header parsed, can release allocated buffer space
	buf = undo

	// TODO copy?
	msg.RawSlots = ctxt[KeySize:msg.OffBody]
	msg.BodyBox = ctxt[msg.OffBody:]

	bodyKey_, err := ReadKey(readKey).DeriveBodyKey(buf, infos, KeySize)
	if err != nil {
		return nil, errors.Wrap(err, "error deriving body key")
	}
	copy(key[:], bodyKey_)

	var plain []byte
	_, plain, buf = alloc(buf, len(msg.BodyBox)-secretbox.Overhead)

	plain, ok = secretbox.Open(plain[:0], msg.BodyBox, &zero24, &key)
	if !ok {
		return nil, fmt.Errorf("body decrypt error")
	}

	return plain, nil
}

func alloc(bs []byte, n int) (old, allocd, new []byte) {
	old, allocd, new = bs, bs[:n], bs[n:]
	return
}

// TODO add padding
