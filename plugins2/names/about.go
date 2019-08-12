package names

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"go.cryptoscope.co/ssb/client"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

type AboutStore interface {
	CollectedFor(*ssb.FeedRef) (*AboutInfo, error)
	ImageFor(*ssb.FeedRef) (*ssb.BlobRef, error)
	All() (client.NamesGetResult, error)
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

func (ab aboutStore) ImageFor(ref *ssb.FeedRef) (*ssb.BlobRef, error) {
	var br ssb.BlobRef

	err := ab.kv.View(func(txn *badger.Txn) error {
		addr := ref.StoredAddr()
		addr += ":"
		addr += ref.StoredAddr()
		addr += ":image"
		it, err := txn.Get([]byte(addr))
		if err != nil {
			return err
		}

		err = it.Value(func(v []byte) error {
			newBlobR, err := ssb.ParseBlobRef(string(v))
			if err != nil {
				return err
			}
			br = *newBlobR
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})

	return &br, err
}

func (ab aboutStore) All() (client.NamesGetResult, error) {
	var ngr = make(client.NamesGetResult)
	err := ab.kv.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()
			k := it.Key()
			if string(k) == "__current_observable" {
				return nil // skip
			}

			parts := strings.Split(string(k), ":")
			if len(parts) != 3 {
				return errors.Errorf("about.All: illegal key:%q", string(k))
			}

			about := parts[0]
			author := parts[1]
			field := parts[2]

			if string(field) == "name" {

				err := it.Value(func(v []byte) error {
					name := string(v)
					name = strings.TrimPrefix(name, "\"")
					name = strings.TrimSuffix(name, "\"")

					abouts, ok := ngr[about]
					if !ok {
						abouts = make(map[string]string)
						abouts[author] = name
						ngr[about] = abouts
						return nil
					}

					abouts[author] = name

					return nil
				})
				if err != nil {
					return errors.Wrapf(err, "about.All: value of item %q failed", k)
				}
			}

		}
		return nil
	})
	return ngr, err
}

func (ab aboutStore) CollectedFor(ref *ssb.FeedRef) (*AboutInfo, error) {
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
			splitted := bytes.Split(k, []byte(":"))

			c, err := ssb.ParseFeedRef(string(splitted[0]))
			if err != nil {
				return errors.Wrapf(err, "about: couldnt make author ref from db key: %s", splitted)
			}
			err = it.Value(func(v []byte) error {
				var fieldPtr *AboutAttribute
				v = bytes.TrimLeft(v, `"`)
				v = bytes.TrimRight(v, `"`)
				foundVal := string(v)
				switch {
				case bytes.HasSuffix(k, []byte(":name")):
					fieldPtr = &reduced.Name
				case bytes.HasSuffix(k, []byte(":description")):
					fieldPtr = &reduced.Description
				case bytes.HasSuffix(k, []byte(":image")):
					fieldPtr = &reduced.Image
				}
				if fieldPtr == nil {
					log.Printf("about debug: %s ", c.Ref())
					log.Printf("no field for: %q", string(k))
					return nil
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

				return nil
			})
			if err != nil {
				return errors.Wrap(err, "about: couldnt get idx value")
			}

		}
		return nil
	})

	return &reduced, errors.Wrap(err, "name db lookup failed")
}

const FolderNameAbout = "about"

func (plug *Plugin) MakeSimpleIndex(r repo.Interface) (librarian.Index, repo.ServeFunc, error) {
	f := func(db *badger.DB) librarian.SinkIndex {
		aboutIdx := libbadger.NewIndex(db, 0)

		return librarian.NewSinkIndex(updateAboutMessage, aboutIdx)
	}

	db, idx, serve, err := repo.OpenBadgerIndex(r, FolderNameAbout, f)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting about index")
	}

	plug.about = aboutStore{db}

	return idx, serve, err
}

func updateAboutMessage(ctx context.Context, seq margaret.Seq, msgv interface{}, idx librarian.SetterIndex) error {
	msg, ok := msgv.(ssb.Message)
	if !ok {
		if margaret.IsErrNulled(msgv.(error)) {
			return nil
		}
		return fmt.Errorf("about(%d): wrong msgT: %T", seq, msgv)
	}

	var aboutMSG ssb.About
	err := json.Unmarshal(msg.ContentBytes(), &aboutMSG)
	if err != nil {
		// fmt.Println("msg", "skipped contact message", "reason", err)
		return nil
		// debugging
		if ssb.IsMessageUnusable(err) {
			return nil
		}
	}

	// about:from:field
	addr := aboutMSG.About.StoredAddr()
	addr += ":"
	addr += msg.Author().StoredAddr()
	addr += ":"

	var val string
	if aboutMSG.Name != "" {
		addr += "name"
		val = aboutMSG.Name
	}
	if aboutMSG.Description != "" {
		addr += "description"
		val = aboutMSG.Description
	}
	if aboutMSG.Image != nil {
		addr += "image"
		val = aboutMSG.Image.Ref()
	}
	if val != "" {
		// fmt.Println("about:", addr, val)
		err = idx.Set(ctx, addr, val)
		if err != nil {
			return errors.Wrap(err, "db/idx about: failed to update field")
		}
	}

	return nil
}
