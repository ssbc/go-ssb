package keys

import (
	"context"
	"fmt"

	"go.cryptoscope.co/librarian"
	refs "go.mindeco.de/ssb-refs"
)

// Q: what's the relation of ID and key?
// A: id is anything we want to use to store and find a key,
// like the id in a database or key in a k:v store.

type Store struct {
	Index librarian.SetterIndex
}

var todoCtx = context.TODO()

func (mgr *Store) AddKey(id ID, r Recipient) error {
	if !r.Scheme.Valid() {
		return Error{Code: ErrorCodeInvalidKeyScheme, Scheme: r.Scheme}
	}

	idxk := &idxKey{
		ks: r.Scheme,
		id: id,
	}

	idxkBytes, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	recps, err := mgr.GetKeys(r.Scheme, id)
	if err != nil {
		if IsNoSuchKey(err) {
			recps = Recipients{}
		} else {
			return fmt.Errorf("error getting old value: %w", err)
		}
	}

	// add new key to existing ones
	recps = append(recps, r)

	return mgr.Index.Set(todoCtx, librarian.Addr(idxkBytes), recps)
}

func (mgr *Store) SetKey(id ID, r Recipient) error {
	if !r.Scheme.Valid() {
		return Error{Code: ErrorCodeInvalidKeyScheme, Scheme: r.Scheme}
	}

	idxk := &idxKey{
		ks: r.Scheme,
		id: id,
	}

	idxkBs, err := idxk.MarshalBinary()
	if err != nil {
		return err
	}

	return mgr.Index.Set(todoCtx, librarian.Addr(idxkBs), Recipients{r})
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

func (mgr *Store) GetKeysForMessage(ks KeyScheme, msg refs.MessageRef) (Recipients, error) {
	panic("TODO")
	// return mgr.getKeys(ks, id)
}

func (mgr *Store) GetKeys(ks KeyScheme, id ID) (Recipients, error) {
	return mgr.getKeys(ks, id)
}

func (mgr *Store) getKeys(ks KeyScheme, id ID) (Recipients, error) {
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

	switch tv := ksIface.(type) {
	case Recipients:
		return tv, nil
	case librarian.UnsetValue:
		return nil, Error{
			Code:   ErrorCodeNoSuchKey,
			Scheme: ks,
			ID:     id,
		}
	default:
		return nil, fmt.Errorf("keys manager: expected type %T, got %T", Recipients{}, ksIface)
	}

}
