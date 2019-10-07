package keys

import (
	"context"
	"encoding/binary"
	"fmt"

	"go.cryptoscope.co/librarian"

	"github.com/pkg/errors"
)

type ErrorCode uint8

const (
	ErrorCodeInternal ErrorCode = iota
	ErrorCodeNoSuchKey
)

func (code ErrorCode) String() string {
	switch code {
	case ErrorCodeInternal:
		return "internal keys error"
	case ErrorCodeNoSuchKey:
		return "no such key"
	default:
		return ""
	}
}

type Error struct {
	Code ErrorCode
	Type Type
	ID   ID

	Cause error
}

func (err Error) Error() string {
	if err.Code == ErrorCodeInternal {
		return err.Cause.Error()
	}

	return fmt.Sprintf("%s at (%s, %x)", err.Code, err.Type, err.ID)
}

func IsNoSuchKey(err error) bool {
	if err_, ok := err.(Error); !ok {
		return false
	} else {
		return err_.Code == ErrorCodeNoSuchKey
	}
}

type Manager struct {
	Index librarian.SetterIndex
}

func (mgr *Manager) AddKey(ctx context.Context, t Type, id ID, key Key) error {
	idxk := &idxKey{
		t:  t,
		id: id,
	}

	idxkBytes, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(key)))

	ks, err := mgr.GetKeys(ctx, t, id)
	if err != nil {
		if IsNoSuchKey(err) {
			ks = Keys{}
		} else {
			return errors.Wrap(err, "error getting old value")
		}
	}

	ks = append(ks, key)

	err = mgr.Index.Set(ctx, librarian.Addr(idxkBytes), ks)

	return err
}

func (mgr *Manager) SetKey(ctx context.Context, t Type, id ID, key Key) error {
	idxk := &idxKey{
		t:  t,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.Index.Set(ctx, librarian.Addr(idxkBs), Keys{key})
}

func (mgr *Manager) RmKeys(ctx context.Context, t Type, id ID) error {
	idxk := &idxKey{
		t:  t,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.Index.Delete(ctx, librarian.Addr(idxkBs))
}

func (mgr *Manager) GetKeys(ctx context.Context, t Type, id ID) (Keys, error) {
	idxk := &idxKey{
		t:  t,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data, err := mgr.Index.Get(ctx, librarian.Addr(idxkBs))
	if err != nil {
		return nil, err
	}

	ksIface, err := data.Value()
	if err != nil {
		return nil, err
	}

	var ks Keys

	switch ksIface.(type) {
	case Keys:
		ks = ksIface.(Keys)
	case librarian.UnsetValue:
		err = Error{
			Code: ErrorCodeNoSuchKey,
			Type: t,
			ID:   id,
		}
	default:
		err = fmt.Errorf("expected type %T, got %T", ks, ksIface)
	}

	return ks, err
}
