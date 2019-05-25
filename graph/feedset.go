package graph

import (
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/ssb"
)

type FeedSet interface {
	AddB([]byte) error
	AddRef(*ssb.FeedRef) error

	List() ([]*ssb.FeedRef, error)
	Has(*ssb.FeedRef) bool
	Count() int
}

type strFeedMap map[librarian.Addr]struct{}

type strFeedSet struct {
	sync.Mutex
	set strFeedMap
}

func NewFeedSet(size int) FeedSet {
	return &strFeedSet{
		set: make(strFeedMap, size),
	}
}

func (fs *strFeedSet) AddB(b []byte) error {
	fs.Lock()
	defer fs.Unlock()

	ref, err := ssb.NewFeedRefEd25519(b)
	if err != nil {
		return err
	}

	fs.set[ref.StoredAddr()] = struct{}{}
	return nil
}

func (fs *strFeedSet) AddRef(ref *ssb.FeedRef) error {
	fs.Lock()
	defer fs.Unlock()
	fs.set[ref.StoredAddr()] = struct{}{}
	return nil
}

func (fs *strFeedSet) Count() int {
	fs.Lock()
	defer fs.Unlock()
	return len(fs.set)
}

func (fs *strFeedSet) List() ([]*ssb.FeedRef, error) {
	fs.Lock()
	defer fs.Unlock()
	var lst = make([]*ssb.FeedRef, len(fs.set))
	i := 0
	for feed := range fs.set {
		var err error
		lst[i], err = ssb.ParseFeedRef(string(feed))
		if err != nil {
			return nil, errors.Wrap(err, "failed to make ref from map entry")
		}
		// log.Printf("dbg List(%d) %s", i, ref.Ref())
		i++
	}
	return lst, nil
}

func (fs *strFeedSet) Has(ref *ssb.FeedRef) bool {
	fs.Lock()
	defer fs.Unlock()
	_, has := fs.set[ref.StoredAddr()]
	return has
}
