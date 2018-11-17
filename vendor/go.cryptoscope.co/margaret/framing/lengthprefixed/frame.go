package lengthprefixed // import "go.cryptoscope.co/margaret/framing/lengthprefixed"

import (
	"encoding/binary"

	"go.cryptoscope.co/margaret"

	"github.com/pkg/errors"
)

var _ margaret.Framing = &frame32{}

// New32 returns a new framing for blocks of size framesize.
// It prefixes the block by the data's length in 32bit big endian format.
func New32(framesize int64) margaret.Framing {
	return &frame32{framesize}
}

type frame32 struct {
	framesize int64
}

func (f *frame32) DecodeFrame(block []byte) ([]byte, error) {
	if int64(len(block)) != f.framesize {
		return nil, errors.New("wrong block size")
	}

	size := int64(binary.BigEndian.Uint32(block[:4]))
	return block[4 : size+4], nil

}

func (f *frame32) EncodeFrame(data []byte) ([]byte, error) {
	if int64(len(data))+4 > f.framesize {
		return nil, errors.New("data too long")
	}

	frame := make([]byte, f.framesize)
	binary.BigEndian.PutUint32(frame[:4], uint32(len(data)))
	copy(frame[4:], data)

	return frame, nil
}

func (f *frame32) FrameSize() int64 {
	return f.framesize
}
