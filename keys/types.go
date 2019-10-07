package keys

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type ID []byte

type Key []byte

type Keys []Key

type Type uint16

func (t Type) String() string {
	switch t {
	case TypeIDSign:
		return "IDSign"
	case TypeIDDH:
		return "IDDH"
	default:
		// catch erroneously unhandled key types
		if t < TypeCount {
			panic(fmt.Sprintf("unhandled key type: %d", t))
		}

		return fmt.Sprintf("(!INVALID KEY TYPE:%d)", t)
	}
}

const (
	TypeIDSign Type = iota
	TypeIDDH
	TypeCount
)

type idxKey struct {
	t  Type
	id ID
}

func (idxk *idxKey) Len() int {
	return 4 + len(idxk.id)
}

func (idxk *idxKey) Read(data []byte) (int64, error) {
	off := 0

	binary.LittleEndian.PutUint16(data[off:], uint16(idxk.t))
	off += 2

	binary.LittleEndian.PutUint16(data[off:], uint16(len(idxk.id)))
	off += 2

	copy(data[off:], []byte(idxk.id))

	return int64(idxk.Len()), nil
}
func (idxk *idxKey) UnmarshalBinary(data []byte) error {
	_, err := idxk.Write(data)
	return err
}

func (idxk *idxKey) Write(data []byte) (int64, error) {
	buf := bytes.NewBuffer(data)

	err := binary.Read(buf, binary.LittleEndian, ((*uint16)(&idxk.t)))
	if err != nil {
		return 0, err
	}

	var length uint16

	err = binary.Read(buf, binary.LittleEndian, &length)
	if err != nil {
		return 0, err
	}

	if len(data) < int(length)+4 {
		return 0, fmt.Errorf("invalid key - claimed id length exceeds buffer")
	}

	if len(idxk.id) < int(length) {
		idxk.id = make(ID, length)
	}

	copy(idxk.id, ID(data[4:]))

	return int64(idxk.Len()), nil
}

func (idxk *idxKey) MarshalBinary() ([]byte, error) {
	data := make([]byte, idxk.Len())
	_, err := idxk.Read(data)
	return data, err
}
