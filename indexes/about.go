package indexes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

type AboutStore interface {
	GetName(*ssb.FeedRef) (*AboutInfo, error)
}

type aboutStore struct {
	kv *badger.DB
}

type AboutInfo struct {
	Name, Description, Image AboutAttribute
}

type AboutAttribute struct {
	Chosen     string
	Prescribed map[string]int
}

func (ab aboutStore) GetName(ref *ssb.FeedRef) (*AboutInfo, error) {
	addr := []byte(ref.StoredAddr() + ":")
	// from self
	// addr = append(addr, ref.ID...)

	// explicit lookup
	// addr = append(addr, []byte(":name")...)
	// if obs, err := ab.idx.Get(context.TODO(), librarian.Addr(addr)); err == nil {
	// 	val, err := obs.Value()
	// 	if err == nil {
	// 		return val.(string)
	// 	}
	// }

	// direct badger magic
	// most of this feels like to direct k:v magic to be honest
	var reduced AboutInfo
	reduced.Name.Prescribed = make(map[string]int)
	reduced.Description.Prescribed = make(map[string]int)
	reduced.Image.Prescribed = make(map[string]int)

	err := ab.kv.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		for iter.Seek(addr); iter.ValidForPrefix(addr); iter.Next() {
			it := iter.Item()
			k := it.Key()
			c, err := ssb.NewFeedRefEd25519(k[len(addr):])
			if err != nil {
				return errors.Wrap(err, "about: counldnt make author ref from db key")
			}
			err = it.Value(func(v []byte) error {
				// log.Printf("about debug: %s ", c.Ref())
				var fieldPtr *AboutAttribute
				foundVal := string(v)
				switch {
				case bytes.HasSuffix(k, []byte(":name")):
					fieldPtr = &reduced.Name
				case bytes.HasSuffix(k, []byte(":description")):
					fieldPtr = &reduced.Description
				case bytes.HasSuffix(k, []byte(":image")):
					fieldPtr = &reduced.Image
				}
				if c.Equal(ref) {
					fieldPtr.Chosen = foundVal
				} else {
					cnt, has := fieldPtr.Prescribed[foundVal]
					if has {
						cnt++
					} else {
						cnt = 1
					}
					fieldPtr.Prescribed[foundVal] = cnt
				}

				// log.Printf(" key: %q", string(k))
				return nil
			})
			if err != nil {
				return errors.Wrap(err, "about: counldnt get idx value")
			}

		}
		return nil
	})

	return &reduced, errors.Wrap(err, "name db lookup failed")
}

const FolderNameAbout = "about"

func OpenAbout(log kitlog.Logger, r repo.Interface) (AboutStore, repo.ServeFunc, error) {
	f := func(db *badger.DB) librarian.SinkIndex {
		aboutIdx := libbadger.NewIndex(db, 0)

		return librarian.NewSinkIndex(updateAboutMessage, aboutIdx)
	}

	db, _, serve, err := repo.OpenBadgerIndex(r, FolderNameAbout, f)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting about index")
	}

	return aboutStore{db}, serve, nil
}

func updateAboutMessage(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
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

	return nil
}
