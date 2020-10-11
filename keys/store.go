package keys

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
)

// Q: what's the relation of ID and key?

type Store struct {
	Index librarian.SetterIndex
}

var todoCtx = context.TODO()

func (mgr *Store) AddKey(ks KeyScheme, id ID, key Key) error {
	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	if !ks.Valid() {
		return Error{Code: ErrorCodeInvalidKeyScheme, Scheme: ks}
	}

	idxkBytes, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	recps, err := mgr.GetKeys(ks, id)
	if err != nil {
		if IsNoSuchKey(err) {
			recps = Recipients{}
		} else {
			return errors.Wrap(err, "error getting old value")
		}
	}

	var keys Keys
	for _, recp := range recps { // convert recps to keys
		keys = append(keys, recp.Key)
	}

	// add new key to existing ones
	keys = append(keys, key)

	return mgr.Index.Set(todoCtx, librarian.Addr(idxkBytes), keys)
}

func (mgr *Store) SetKey(ks KeyScheme, id ID, key Key) error {
	if !ks.Valid() {
		return Error{Code: ErrorCodeInvalidKeyScheme, Scheme: ks}
	}

	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.Index.Set(todoCtx, librarian.Addr(idxkBs), Keys{key})
}

func (mgr *Store) RmKeys(ks KeyScheme, id ID) error {
	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.Index.Delete(todoCtx, librarian.Addr(idxkBs))
}

func (mgr *Store) GetKeys(ks KeyScheme, id ID) (Recipients, error) {
	if !ks.Valid() {
		return nil, Error{Code: ErrorCodeInvalidKeyScheme, Scheme: ks}
	}

	idxk := &idxKey{
		ks: ks,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data, err := mgr.Index.Get(todoCtx, librarian.Addr(idxkBs))
	if err != nil {
		return nil, err
	}

	ksIface, err := data.Value()
	if err != nil {
		return nil, err
	}

	var recps Recipients

	switch ksIface.(type) {
	case Keys:
		for _, k := range ksIface.(Keys) {
			recps = append(recps, Recipient{
				Key:    k,
				Scheme: ks,
			})
		}
	case librarian.UnsetValue:
		return nil, Error{
			Code:   ErrorCodeNoSuchKey,
			Scheme: ks,
			ID:     id,
		}
	default:
		return nil, fmt.Errorf("keys manager: expected type %T, got %T", recps, ksIface)
	}

	return recps, nil
}
