package indexes

import (
	"context"

	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb/keys"
	"go.cryptoscope.co/ssb/repo"
)

const FolderNameKeys = "keys"

func OpenKeys(log kitlog.Logger, r repo.Interface) (*keys.Manager, librarian.Index, repo.ServeFunc, error) {
	var mgr *keys.Manager

	f := func(idx librarian.SeqSetterIndex) librarian.SinkIndex {
		// HACK (I think): f is called in OpenIndex below,
		// so be the time we return, mgr is set
		mgr = &keys.Manager{idx}

		return librarian.NewSinkIndex(updateKeyManager, idx)
	}

	// HACK: within this function, f is called
	idx, serve, err := repo.OpenIndex(r, FolderNameKeys, f)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error getting about index")
	}

	return mgr, idx, serve, nil
}

func updateKeyManager(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
	// Remember: We can instantiate a key manager here from idx.
	// TODO:
	// - parse entrust messages and add the keys to the manager

	/*
		msg, ok := val.(ssb.Message)
		if !ok {
			if margaret.IsErrNulled(val.(error)) {
				return nil
			}
			return fmt.Errorf("about(%d): wrong msgT: %T", seq, val)
		}

		var aboutMSG ssb.About
		err := json.Unmarshal(msg.ContentBytes(), &aboutMSG)
		if err != nil {
			if ssb.IsMessageUnusable(err) {
				return nil
			}
			// log.Log("msg", "skipped contact message", "reason", err)
			return nil
		}

		// about:from:field
		addr := aboutMSG.About.StoredAddr()
		addr += ":"
		addr += msg.Author().StoredAddr()
		addr += ":"
		if aboutMSG.Name != "" {
			err = idx.Set(ctx, addr+"name", aboutMSG.Name)
		}
		if aboutMSG.Description != "" {
			err = idx.Set(ctx, addr+"description", aboutMSG.Description)
		}
		if aboutMSG.Image != nil {
			err = idx.Set(ctx, addr+"image", aboutMSG.Image.Ref())
		}
		if err != nil {
			return errors.Wrap(err, "db/idx about: failed to update field")
		}
	*/

	return nil
}
