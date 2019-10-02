package keys

import (
	"encoding/binary"
	"fmt"

	"modernc.org/kv"
)

type Manager interface {
	SetKey(Type, ID, Key) error
	AddKey(Type, ID, Key) error
	RmKey(Type, ID) error
	Query(Type, ID) ([]Key, error)
}

type manager struct {
	db *kv.DB
}

func (mgr *manager) AddKey(t Type, id ID, key Key) error {
	dbk := &dbKey{
		t: t,
		id: id,
	}

	dbkBytes, err := dbk.MarshalBinary()
	if err != nil {
		return err
	}

	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(key)))

	_, _, err = mgr.db.Put(nil, dbkBytes, func(_, old []byte) ([]byte, bool, error) {
		var new []byte

		if len(old) == 0 {
			new = []byte{0,0}
		} else {
			new = make([]byte, len(old), len(old) + 2 + len(key))
			copy(new, old)
		}

		count := binary.LittleEndian.Uint16(new[:2]) + 1
		binary.LittleEndian.PutUint16(new[:2], count)

		new = append(new, lenBuf[:]...)
		new = append(new, []byte(key)...)

		return new, true, nil
	})

	return err
}

func (mgr *manager) SetKey(t Type, id ID, key Key) error {
	dbk := &dbKey{
		t: t,
		id: id,
	}

	dbkBs, err := dbk.MarshalBinary()
	if err != nil {
		return err
	}

	// uint16LE key count || unit16LE key length || key
	keyBs := make([]byte, 2 + 2 + len(key))
	binary.LittleEndian.PutUint16(keyBs[:2], uint16(1))
	binary.LittleEndian.PutUint16(keyBs[2:4], uint16(len(key)))
	copy(keyBs[4:], key)

	return mgr.db.Set(dbkBs, keyBs)
}

func (mgr *manager) RmKeys(t Type, id ID) error {
	dbk := &dbKey{
		t: t,
		id: id,
	}

	dbkBs, err := dbk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.db.Delete(dbkBs)
}

func (mgr *manager) GetKeys(t Type, id ID) (*Keys, error) {
	dbk := &dbKey{
		t: t,
		id: id,
	}

	dbkBs, err := dbk.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data, err := mgr.db.Get(nil, dbkBs)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return nil, fmt.Errorf("no such key")
	}

	ks := &Keys{}

	_, err = ks.Write(data)
	return ks, err
}
