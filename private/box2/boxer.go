package box2

import (
	"encoding/binary"
	stderr "errors"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/crypto/nacl/secretbox"

	"go.cryptoscope.co/ssb/keys"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

const (
	KeySize = 256 / 8

	MaxSlots = 32
)

var (
	zero24  [24]byte
	zeroKey [KeySize]byte
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
	rand io.Reader
}

type makeHKDFContextList func(...[]byte) [][]byte

func makeInfo(author *refs.FeedRef, prev *refs.MessageRef) (makeHKDFContextList, error) {
	if prev == nil {
		if author.Algo != refs.RefAlgoFeedSSB1 {
			return nil, fmt.Errorf("unsupported feed type: %s", author.Algo)
		}
		prev = &refs.MessageRef{
			Algo: refs.RefAlgoMessageSSB1,
			Hash: make([]byte, 32),
		}
	}

	tfkFeed, err := tfk.FeedFromRef(author)
	if err != nil {
		return nil, err
	}
	feedBytes, err := tfkFeed.MarshalBinary()
	if err != nil {
		return nil, err
	}

	tfkMsg, err := tfk.MessageFromRef(prev)
	if err != nil {
		return nil, err
	}
	msgBytes, err := tfkMsg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return func(infos ...[]byte) [][]byte {
		out := make([][]byte, len(infos)+3)
		out[0] = []byte("envelope")
		out[1] = feedBytes
		out[2] = msgBytes
		copy(out[3:], infos)
		return out
	}, nil
}

// API and processing errors
var (
	ErrTooManyRecipients = stderr.New("box2: too many recipients")
	ErrCouldNotDecrypt   = stderr.New("box2: could not decrypt")
	ErrInvalid           = stderr.New("box2: message is invalid")
	ErrEmptyPlaintext    = stderr.New("box2: won't encrypt empty plaintext")
	ErrInvalidOffset     = stderr.New("box2: precalculated body offset does not match real body offset")
)

// Encrypt takes a buffer to write into (out), the plaintext to encrypt, the (author) of the message, her (prev)ious message hash and a list of recipients (recpts).
// If out is too small to hold the full message, additonal allocations will be made. The ciphertext is returned as the first return value.
func (bxr *Boxer) Encrypt(out, plain []byte, author *refs.FeedRef, prev *refs.MessageRef, recpts []keys.Recipient) ([]byte, error) {
	if len(plain) == 0 {
		return nil, ErrEmptyPlaintext
	}

	if len(recpts) > MaxSlots {
		return nil, ErrTooManyRecipients
	}

	var (
		msgKey    [KeySize]byte
		readKey   [KeySize]byte
		bodyKey   [KeySize]byte
		headerKey [KeySize]byte
		slotKey   [KeySize]byte

		// header length + len(rceps) * slot length
		bodyOff uint16 = 32 + uint16(len(recpts))*32

		// header plaintext
		headerPlain [16]byte
	)

	_, err := bxr.rand.Read(msgKey[:])
	if err != nil {
		return nil, errors.Wrap(err, "error reading random data")
	}

	info, err := makeInfo(author, prev)
	if err != nil {
		return nil, errors.Wrap(err, "error constructing keying information")
	}

	deriveTo(readKey[:], msgKey[:], info([]byte("read_key"))...)

	// build header plaintext
	binary.LittleEndian.PutUint16(headerPlain[:], bodyOff)

	// append header ciphertext
	deriveTo(headerKey[:], readKey[:], info([]byte("header_key"))...)
	out = secretbox.Seal(out, headerPlain[:], &zero24, &headerKey)
	clear(headerKey[:])

	// append slots
	for _, bk := range recpts {
		deriveTo(slotKey[:], bk.Key, info([]byte("slot_key"), []byte(bk.Scheme))...)

		out = append(out, make([]byte, KeySize)...)
		for i := range slotKey {
			out[len(out)-KeySize+i] = slotKey[i] ^ msgKey[i]
		}
	}
	clear(msgKey[:])

	// let's not spread broken messages
	if len(out) != int(bodyOff) {
		return nil, ErrInvalidOffset
	}

	// append encrypted body
	deriveTo(bodyKey[:], readKey[:], info([]byte("body_key"))...)
	out = secretbox.Seal(out, plain, &zero24, &bodyKey)
	clear(bodyKey[:])
	clear(readKey[:])

	return out, nil
}

func deriveMessageKey(author *refs.FeedRef, prev *refs.MessageRef, candidates []keys.Recipient) ([][KeySize]byte, makeHKDFContextList, error) {
	var slotKeys = make([][KeySize]byte, len(candidates))

	info, err := makeInfo(author, prev)
	if err != nil {
		return nil, nil, err
	}

	// derive slot keys
	for i, candidate := range candidates {
		deriveTo(slotKeys[i][:], candidate.Key, info([]byte("slot_key"), []byte(candidate.Scheme))...)
	}

	return slotKeys, info, nil
}

// TODO: Maybe return entire decrypted message?
func (bxr *Boxer) Decrypt(out, ctxt []byte, author *refs.FeedRef, prev *refs.MessageRef, candidates []keys.Recipient) ([]byte, error) {
	slotKeys, info, err := deriveMessageKey(author, prev, candidates)
	if err != nil {
		return nil, errors.Wrap(err, "error constructing keying information")
	}
	var (
		hdr               = make([]byte, 16)
		msgKey, headerKey [KeySize]byte
		readKey, bodyKey  [KeySize]byte
		slot              []byte
		ok                bool
		i, j, k           int

		headerbox   = ctxt[:32]
		afterHeader = ctxt[32:]
	)

	// find correct slot key and decrypt header
OUTER:
	for i = 0; (i+1)*KeySize < len(afterHeader) && i < MaxSlots; i++ {
		slot = afterHeader[i*KeySize : (i+1)*KeySize]

		for j = 0; j < len(slotKeys); j++ {
			for k = range slotKeys[j] {
				msgKey[k] = slotKeys[j][k] ^ slot[k]
			}

			deriveTo(readKey[:], msgKey[:], info([]byte("read_key"))...)
			deriveTo(headerKey[:], readKey[:], info([]byte("header_key"))...)

			hdr, ok = secretbox.Open(hdr[:0], headerbox, &zero24, &headerKey)
			if ok {
				break OUTER
			}
		}
	}
	if !ok {
		return nil, ErrCouldNotDecrypt
	}

	var (
		bodyOffset = int(binary.LittleEndian.Uint16(hdr))
		plain      = make([]byte, 0, len(ctxt)-bodyOffset-secretbox.Overhead)
	)

	// decrypt body
	deriveTo(bodyKey[:], readKey[:], info([]byte("body_key"))...)
	plain, ok = secretbox.Open(plain, ctxt[bodyOffset:], &zero24, &bodyKey)
	if !ok {
		return nil, ErrInvalid
	}

	return plain, nil
}

// utils
func clear(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
