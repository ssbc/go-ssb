package graph

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/ssb"
)

type FeedSet interface {
	AddB([]byte) error
	AddRef(*ssb.FeedRef) error
	AddAddr(librarian.Addr) error

	List() ([]*ssb.FeedRef, error)
	Has(*ssb.FeedRef) bool
	Count() int
}

type feedMap map[[32]byte]struct{}

type feedSet struct {
	sync.Mutex
	set feedMap
}

func NewFeedSet(size int) FeedSet {
	return &feedSet{
		set: make(feedMap, size),
	}
}

type errKeyLen int

func (e errKeyLen) Error() string {
	return fmt.Sprintf("invalid keylen for feedSet: %d", int(e))
}

func (fs *feedSet) AddB(b []byte) error {
	fs.Lock()
	defer fs.Unlock()
	if n := len(b); n != 32 {
		return errKeyLen(n)
	}
	k, err := copyKeyBytes(b)
	if err != nil {
		return err
	}
	fs.set[k] = struct{}{}
	return nil
}

func (fs *feedSet) AddAddr(addr librarian.Addr) error {
	fs.Lock()
	defer fs.Unlock()
	if n := len(addr); n != 32 {
		return errKeyLen(n)
	}
	k, err := copyKeyBytes([]byte(addr))
	if err != nil {
		return err
	}
	fs.set[k] = struct{}{}
	return nil
}

func (fs *feedSet) AddRef(ref *ssb.FeedRef) error {
	fs.Lock()
	defer fs.Unlock()
	if n := len(ref.ID); n != 32 {
		return errKeyLen(n)
	}
	k, err := copyKeyBytes(ref.ID)
	if err != nil {
		return err
	}
	fs.set[k] = struct{}{}
	return nil
}

func copyKeyBytes(b []byte) ([32]byte, error) {
	var k [32]byte
	if n := copy(k[:], b); n != 32 {
		return k, errKeyLen(n)
	}
	return k, nil
}

func (fs feedSet) Count() int {
	fs.Lock()
	defer fs.Unlock()
	return len(fs.set)
}

func (fs feedSet) List() ([]*ssb.FeedRef, error) {
	fs.Lock()
	defer fs.Unlock()
	var lst = make([]*ssb.FeedRef, len(fs.set))
	i := 0
	for feed := range fs.set {
		ref, err := ssb.NewFeedRefEd25519(feed)
		if err != nil {
			return nil, errors.Wrap(err, "failed to make ref from map entry")
		}
		// log.Printf("dbg List(%d) %s", i, ref.Ref())
		lst[i] = ref
		i++
	}
	return lst, nil
}

func (fs feedSet) Has(ref *ssb.FeedRef) bool {
	fs.Lock()
	defer fs.Unlock()
	var k [32]byte
	copy(k[:], ref.ID)
	_, has := fs.set[k]
	return has
}
