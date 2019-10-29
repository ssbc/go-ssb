package keys

import (
	"encoding/binary"
	"fmt"
)

type ID []byte

type Type string

type idxKey struct {
	t  Type
	id ID
}

func (idxk *idxKey) Len() int {
	return 4 + len(idxk.id) + len(idxk.t)
}

func (idxk *idxKey) Read(data []byte) (int64, error) {
	if len(data) < idxk.Len() {
		return 0, fmt.Errorf("buffer too short: need %d, got %d", idxk.Len(), len(data))
	}

	var off int

	binary.LittleEndian.PutUint16(data[off:], uint16(len(idxk.t)))
	off += 2

	copy(data[off:], []byte(idxk.t))
	off += len(idxk.t)

	binary.LittleEndian.PutUint16(data[off:], uint16(len(idxk.id)))
	off += 2

	copy(data[off:], []byte(idxk.id))

	return int64(idxk.Len()), nil
}

func (idxk *idxKey) MarshalBinary() ([]byte, error) {
	data := make([]byte, idxk.Len())
	_, err := idxk.Read(data)
	return data, err
}

func (idxk *idxKey) Write(data []byte) (int64, error) {
	var off int

	if diff := len(data) - off; diff < 2 {
		return 0, fmt.Errorf("data too short to read type length")
	}

	typeLen := binary.LittleEndian.Uint16(data[0:])
	off += 2

	if diff := len(data) - off; diff < int(typeLen)+2 {
		return 0, fmt.Errorf("invalid key - claimed type length exceeds buffer")
	}

	idxk.t = Type(data[off : off+int(typeLen)])
	off += int(typeLen)

	idLen := binary.LittleEndian.Uint16(data[off:])
	off += 2

	if diff := len(data) - off; diff < int(idLen) {
		return 0, fmt.Errorf("invalid key - claimed id length exceeds buffer")
	}

	if len(idxk.id) < int(idLen) {
		idxk.id = make(ID, idLen)
	}

	copy(idxk.id, ID(data[off:]))

	return int64(idxk.Len()), nil
}

func (idxk *idxKey) UnmarshalBinary(data []byte) error {
	_, err := idxk.Write(data)
	return err
}
