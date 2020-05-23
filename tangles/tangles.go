package tangles

import (
	"context"
	"encoding/binary"
	"encoding/json"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/margaret/multilog/abstractkv"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/errors"
	"go.cryptoscope.co/ssb/message/msgkit"
)

type Record struct {
	Root     *ssb.MessageRef   `json:"root"`
	Previous []*ssb.MessageRef `json:"previous"`
}

type previousList []*ssb.MessageRef

type Service struct {
	fullThreads multilog.MultiLog
	prevs       abstractkv.Store
	prevsCodec  margaret.Codec
}

var (
	_ luigi.Sink = &Sink{}
)

func (srvc *Service) Get(name string, root *ssb.MessageRef) (*Thread, error) {
	addr, err := refAddr(name, root)
	if err != nil {
		return nil, errors.Wrap(err, "error getting address for (name, root) tuple")
	}

	fullThread, err := srvc.fullThreads.Get(addr)
	if err != nil {
		return nil, errors.Wrap(err, "error getting full thread log")
	}

	prevsBs, err := srvc.prevs.Get(abstractkv.Key(addr))
	if err != nil {
		return nil, errors.Wrap(err, "error getting previous links")
	}

	prevs, err := srvc.prevsCodec.Unmarshal(prevsBs)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing previous links")
	}

	return &Thread{
		root:  root,
		name:  name,
		full:  fullThread,
		prevs: luigi.NewObservable(prevs),
	}, nil
}

type Thread struct {
	root *ssb.MessageRef
	name string

	full      margaret.Log
	prevs     luigi.Observable
	prevStore abstractkv.Store
	codec     margaret.Codec
}

func (td *Thread) Root() *ssb.MessageRef {
	return td.root
}

func (td *Thread) Name() string {
	return td.name
}

func (td *Thread) Query(opts ...margaret.QuerySpec) (luigi.Source, error) {
	// TODO figure out if it makes sense to introduce new, thread-specific options
	//      e.g.: messages that are causally after msg x but before msg y
	return td.full.Query(opts...)
}

func (td *Thread) Plan() (msgkit.Plan, error) {
	var (
		prevsIface interface{}
		prevs      []*ssb.MessageRef
		path       = []string{"tangles", td.name}
		err        error
		ok         bool
	)

	prevsIface, err = td.prevs.Value()
	if err != nil {
		return nil, errors.Wrap(err, "error getting previous links")
	}

	prevs, ok = prevsIface.([]*ssb.MessageRef)
	if !ok {
		return nil, errors.TypeError{Expected: prevs, Actual: prevsIface}
	}

	return func(msg *msgkit.Builder) error {
		return msg.Set(path, Record{
			Root:     td.root,
			Previous: prevs,
		})
	}, nil
}

func (td *Thread) setPrevs(prevs []*ssb.MessageRef) error {
	prevsBs, err := td.codec.Marshal(prevs)
	if err != nil {
		return errors.Wrap(err, "error marshaling previous links")
	}

	key, err := refKey(td.name, td.root)
	if err != nil {
		return errors.Wrap(err, "error computing key for (name, root) tuple")
	}
	err = td.prevStore.Put(key, prevsBs)
	return errors.Wrap(err, "error putting previous links")
}

type Sink Service

func (sink *Sink) Pour(ctx context.Context, v interface{}) error {
	// decode message
	sw, ok := v.(margaret.SeqWrapper)
	if !ok {
		return errors.TypeError{Expected: sw, Actual: v}
	}

	var (
		srvc    = (*Service)(sink)
		seq     = sw.Seq()
		vMsg    = sw.Value()
		msg     ssb.Message
		threads = make(map[string]Record)
		old     []*ssb.MessageRef
	)

	msg, ok = vMsg.(ssb.Message)
	if !ok {
		return errors.TypeError{Expected: msg, Actual: vMsg}
	}

	var content = struct {
		Tangles map[string]Record `json:"tangles"`
	}{threads}

	err := json.Unmarshal(msg.ValueContent().Content, &content)
	if err != nil {
		return errors.Wrap(err, "error decoding tangle content")
	}

	for name, rec := range threads {
		// add message to tangle logs
		thread, err := srvc.Get(name, rec.Root)
		if err != nil {
			return errors.Wrapf(err, "error getting thread for (name: %q, root: %q)", name, rec.Root.Ref())
		}

		_, err = thread.full.Append(seq)
		if err != nil {
			return errors.Wrap(err, "error adding message to thread")
		}

		// update heads
		prevsIface, err := thread.prevs.Value()
		if err != nil {
			return errors.Wrap(err, "error getting previous links")
		}

		prevs, ok := prevsIface.([]*ssb.MessageRef)
		if !ok {
			return errors.TypeError{Expected: prevs, Actual: prevsIface}
		}

		var iSrc, iDst int

	OUTER:
		for iSrc = range prevs {
			// skip refs that are in the message
			for _, inMsg := range rec.Previous {
				// TODO find a better way to check equality?
				if old[iSrc].Ref() == inMsg.Ref() {
					continue OUTER
				}
			}

			prevs[iDst] = prevs[iSrc]
			iDst++
		}

		prevs = prevs[:iDst]
		prevs = append(prevs, msg.Key())

		err = thread.setPrevs(prevs)
		if err != nil {
			return errors.Wrap(err, "cound not set new prevs")
		}
	}

	return nil
}

// TODO
func (sink *Sink) Close() error { return nil }

func refAddr(name string, root *ssb.MessageRef) (librarian.Addr, error) {
	key, err := refKey(name, root)
	return librarian.Addr(key), err
}

func refKey(name string, root *ssb.MessageRef) (abstractkv.Key, error) {
	sref, err := ssb.NewStorageRef(root)
	if err != nil {
		return nil, errors.Wrap(err, "could not make storage ref for message ref")
	}
	bref, err := sref.Marshal()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal storage ref")
	}

	lName := len(name)
	lRef := len(bref)

	key := make(abstractkv.Key, 2+lName+2+lRef)

	var off int

	binary.LittleEndian.PutUint16(key[off:], uint16(lName))
	off += 2

	off += copy(key[off:], []byte(name))

	binary.LittleEndian.PutUint16(key[off:], uint16(lRef))
	off += 2

	off += copy(key[off:], bref)
	return key, nil
}
