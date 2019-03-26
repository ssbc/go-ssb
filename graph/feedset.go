package graph

import (
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/ssb"
)

type FeedSet interface {
	AddRef(*ssb.FeedRef)
	AddAddr(librarian.Addr)

	List() []*ssb.FeedRef
	Has(*ssb.FeedRef) bool
	Count() int
}

type feedMap map[[32]byte]struct{}

type feedSet struct {
	set feedMap
}

func NewFeedSet(size int) FeedSet {
	return &feedSet{
		set: make(feedMap, size),
	}
}

func (fs *feedSet) addB(b []byte) {
	if len(b) != 32 {
		panic("invalid key for feedSet")
	}
	var ref [32]byte
	copy(ref[:], b)
	// log.Print("addingB", ref)
	fs.set[ref] = struct{}{}
}

func (fs *feedSet) AddAddr(addr librarian.Addr) {
	if len(addr) != 32 {
		panic("invalid addr for feedSet")
	}
	var ref [32]byte
	copy(ref[:], addr)
	// log.Print("addingB", ref)
	fs.set[ref] = struct{}{}

}

func (fs *feedSet) AddRef(ref *ssb.FeedRef) {
	// log.Print("adding", ref.Ref())
	if len(ref.ID) != 32 {
		panic("invalid key for feedSet")
	}
	var k [32]byte
	copy(k[:], ref.ID)
	fs.set[k] = struct{}{}
}

func (fs feedSet) Count() int {
	return len(fs.set)
}
func (fs feedSet) List() []*ssb.FeedRef {
	var lst = make([]*ssb.FeedRef, len(fs.set))
	i := 0
	for feed := range fs.set {
		ref := ssb.NewFeedRefEd25519(feed)
		// log.Printf("dbg List(%d) %s", i, ref.Ref())
		lst[i] = ref
		i++
	}
	return lst
}

func (fs feedSet) Has(ref *ssb.FeedRef) bool {
	var k [32]byte
	copy(k[:], ref.ID)
	_, has := fs.set[k]
	return has
}
