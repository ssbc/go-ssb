package keys

import (
	"encoding/binary"
	"bytes"
	"fmt"
)

type ID []byte

type Key []byte

type Keys []Key

func (ks *Keys) Read(data []byte) (int64, error) {
	var (
		count = uint16(len(*(*[]Key)(ks)))
	)

	sz := ks.Len()

	if len(data) < sz {
		return 0, fmt.Errorf("buffer too small, need %d", sz)
	}

	buf := bytes.NewBuffer(data)

	err := binary.Write(buf, binary.LittleEndian, count)
	if err != nil {
		return 0, err
	}

	off := 2

	for _, k := range *ks {
		binary.Write(buf, binary.LittleEndian, uint16(len(k)))
		buf.Write([]byte(k))
		off += 2 + len(k)
	}

	if off != sz {
		fmt.Printf("mismatch - off:%d sz:%d\n", off, sz)
	}

	return int64(sz), nil
}

func (ks *Keys) Write(data []byte) (int64, error) {
	var off int64

	count := binary.LittleEndian.Uint16(data[off:])
	off += 2

	var (
		ksz uint16
	)
		
	for count > 0 {
		if len(data[off:]) < 2 {
			return 0, fmt.Errorf("could not read header")
		}
		ksz = binary.LittleEndian.Uint16(data[off:])
		off += 2

		k := make([]byte, int(ksz))
		copy(k, data[off:])	
		off += int64(ksz)
		*ks = append(*ks, Key(k))
		count--
	}

	return off, nil
}

func (ks *Keys) Len() int {
	sz := 2 * (1 + len(*ks))

	for _, k := range *ks {
		sz += len(k)
	}

	return sz
}

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

type dbKey struct {
	t Type
	id ID
}

func (dbk *dbKey) Len() int {
	return 4 + len(dbk.id)
}

func (dbk *dbKey) Read(data []byte) (int64, error) {
	off := 0

	binary.LittleEndian.PutUint16(data[off:], uint16(dbk.t))
	off +=2

	binary.LittleEndian.PutUint16(data[off:], uint16(len(dbk.id)))
	off +=2

	copy(data[off:], []byte(dbk.id))

	return int64(dbk.Len()), nil
}
func (dbk *dbKey) UnmarshalBinary(data []byte) error {
	_, err := dbk.Write(data)
	return err
}

func (dbk *dbKey) Write(data []byte) (int64, error) {
	buf := bytes.NewBuffer(data)

	err := binary.Read(buf, binary.LittleEndian, ((*uint16)(&dbk.t)))
	if err != nil {
		return 0, err
	}

	var length uint16

	err = binary.Read(buf, binary.LittleEndian, &length)
	if err != nil {
		return 0, err
	}

	if len(data) < int(length) + 4 {
		return 0, fmt.Errorf("invalid key - claimed id length exceeds buffer")
	}

	if len(dbk.id) < int(length) {
		dbk.id = make(ID, length)
	}

	copy(dbk.id, ID(data[4:]))

	return int64(dbk.Len()), nil
}

func (dbk *dbKey) MarshalBinary() ([]byte, error) {
	data := make([]byte, dbk.Len())
	_, err := dbk.Read(data)
	return data, err
}
