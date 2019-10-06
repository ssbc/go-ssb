package keys

import (
	"encoding/binary"
	"fmt"

	"modernc.org/kv"
)

type Manager struct {
	idx librarian.Index
}

func (mgr *Manager) AddKey(t Type, id ID, key Key) error {
	idxk := &idxKey{
		t: t,
		id: id,
	}

	idxkBytes, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(key)))

	_, _, err = mgr.DB.Put(nil, idxkBytes, func(_, old []byte) ([]byte, bool, error) {
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

func (mgr *Manager) SetKey(t Type, id ID, key Key) error {
	idxk := &idxKey{
		t: t,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	// uint16LE key count || unit16LE key length || key
	keyBs := make([]byte, 2 + 2 + len(key))
	binary.LittleEndian.PutUint16(keyBs[:2], uint16(1))
	binary.LittleEndian.PutUint16(keyBs[2:4], uint16(len(key)))
	copy(keyBs[4:], key)

	return mgr.DB.Set(idxkBs, keyBs)
}

func (mgr *Manager) RmKeys(t Type, id ID) error {
	idxk := &idxKey{
		t: t,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.DB.Delete(idxkBs)
}

func (mgr *Manager) GetKeys(t Type, id ID) (*Keys, error) {
	idxk := &idxKey{
		t: t,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data, err := mgr.DB.Get(nil, idxkBs)
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
