package blobstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	goon "github.com/shurcooL/go-goon"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
)

func dump(log logging.Interface, v interface{}, from string) {
	var x interface{}
	if msg, ok := v.(WantMsg); ok {
		x = &msg
	}

	if msg, ok := v.(*WantMsg); ok {
		m := make(map[string]int64)
		for _, w := range *msg {
			m[w.Ref.Ref()] = w.Dist
		}
		x = m
	}

	for _, str := range strings.Split(goon.Sdump(x), "\n") {
		log.Log("from", from, "goon", strings.Replace(str, "\t", "  ", -1))
	}
}

func NewWantManager(log logging.Interface, bs sbot.BlobStore) sbot.WantManager {
	wmgr := &wantManager{
		bs:    bs,
		wants: make(map[string]int64),
		info:  log,
	}

	wmgr.wantSink, wmgr.Broadcast = luigi.NewBroadcast()

	bs.Changes().Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		wmgr.l.Lock()
		defer wmgr.l.Unlock()

		n, ok := v.(sbot.BlobStoreNotification)
		if ok && n.Op == sbot.BlobStoreOpPut {
			if _, ok := wmgr.wants[n.Ref.Ref()]; ok {
				delete(wmgr.wants, n.Ref.Ref())
			}
		}

		return nil
	}))

	return wmgr
}

type wantManager struct {
	luigi.Broadcast

	bs sbot.BlobStore

	wants    map[string]int64
	wantSink luigi.Sink

	l sync.Mutex

	info logging.Interface
}

func (wmgr *wantManager) Wants(ref *sbot.BlobRef) bool {
	wmgr.l.Lock()
	defer wmgr.l.Unlock()

	_, ok := wmgr.wants[ref.Ref()]
	return ok
}

func (wmgr *wantManager) Want(ref *sbot.BlobRef) error {
	return wmgr.WantWithDist(ref, -1)
}

func (wmgr *wantManager) WantWithDist(ref *sbot.BlobRef, dist int64) error {
	wmgr.info.Log("func", "WantWithDist", "dist", dist)
	f, err := wmgr.bs.Get(ref)
	if err == nil {
		wmgr.info.Log("func", "WantWithDist", "available", true)
		return f.(io.Closer).Close()
	}

	wmgr.info.Log("func", "WantWithDist", "available", false)
	wmgr.l.Lock()
	defer wmgr.l.Unlock()

	wmgr.wants[ref.Ref()] = dist

	err = wmgr.wantSink.Pour(context.TODO(), want{ref, dist})
	err = errors.Wrap(err, "error pouring want to broadcast")
	return err
}

func (wmgr *wantManager) CreateWants(ctx context.Context, sink luigi.Sink, edp muxrpc.Endpoint) luigi.Sink {
	proc := &wantProc{
		bs:          wmgr.bs,
		wmgr:        wmgr,
		out:         sink,
		remoteWants: make(map[string]int64),
		edp:         edp,
	}

	proc.init()

	return proc
}

type want struct {
	Ref *sbot.BlobRef

	// if Dist is negative, it is the hop count to the original wanter.
	// if it is positive, it is the size of the blob.
	Dist int64
}

type wantProc struct {
	l sync.Mutex

	bs          sbot.BlobStore
	wmgr        *wantManager
	out         luigi.Sink
	remoteWants map[string]int64
	done        func(func())
	edp         muxrpc.Endpoint
}

func (proc *wantProc) init() {
	bsCancel := proc.bs.Changes().Register(
		luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			proc.l.Lock()
			defer proc.l.Unlock()

			notif := v.(sbot.BlobStoreNotification)
			proc.wmgr.info.Log("event", "wantProc notification", "op", notif.Op, "ref", notif.Ref.Ref())
			_, ok := proc.remoteWants[notif.Ref.Ref()]
			if ok {
				sz, err := proc.bs.Size(notif.Ref)
				if err != nil {
					return errors.Wrap(err, "error getting blob size")
				}

				m := map[string]int64{notif.Ref.Ref(): sz}
				err = proc.out.Pour(ctx, m)
				proc.wmgr.info.Log("event", "createWants.Out", "cause", "changesnotification")
				dump(proc.wmgr.info, m, "out sink")
				return errors.Wrap(err, "errors pouring into sink")
			}

			return nil
		}))

	wmCancel := proc.wmgr.Register(
		luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			w := v.(want)
			proc.wmgr.info.Log("op", "sending want we now want", "want", w.Ref.Ref())
			return proc.out.Pour(ctx, WantMsg{w})
		}))

	oldDone := proc.done
	proc.done = func(next func()) {
		bsCancel()
		wmCancel()
		if oldDone != nil {
			oldDone(nil)
		}
	}

	err := proc.out.Pour(context.TODO(), proc.wmgr.wants)
	proc.wmgr.info.Log("event", "createWants.Out", "cause", "initial wants")
	dump(proc.wmgr.info, proc.wmgr.wants, "after pour")
	if err != nil {
		proc.wmgr.info.Log("event", "wantProc.init/Pour", "err", err.Error())
	}
}

func (proc *wantProc) Close() error {
	defer proc.done(nil)
	return errors.Wrap(proc.out.Close(), "error in lower-layer close")
}

func (proc *wantProc) Pour(ctx context.Context, v interface{}) error {
	proc.wmgr.info.Log("event", "createWants.In", "cause", "received data")
	dump(proc.wmgr.info, v, "proc input")
	proc.l.Lock()
	defer proc.l.Unlock()
	dump(proc.wmgr.info, proc.wmgr.wants, "our wants before processing")

	mIn := v.(*WantMsg)
	mOut := make(map[string]int64)

	for _, w := range *mIn {
		if w.Dist < 0 {
			s, err := proc.bs.Size(w.Ref)
			if err != nil {
				if err == ErrNoSuchBlob {
					proc.remoteWants[w.Ref.Ref()] = w.Dist - 1
					continue
				}

				return errors.Wrap(err, "error getting blob size")
			}

			delete(proc.remoteWants, w.Ref.Ref())
			mOut[w.Ref.Ref()] = s
		} else {
			if proc.wmgr.Wants(w.Ref) {
				proc.wmgr.info.Log("event", "createWants.In", "msg", "peer has blob we want", "ref", w.Ref.Ref())
				go func(ref *sbot.BlobRef) {
					src, err := proc.edp.Source(ctx, &WantMsg{}, muxrpc.Method{"blobs", "get"}, ref.Ref())
					if err != nil {
						proc.wmgr.info.Log("event", "blob fetch err", "ref", ref.Ref(), "error", err.Error())
						return
					}

					r := muxrpc.NewSourceReader(src)
					newBr, err := proc.bs.Put(r)
					if err != nil {
						proc.wmgr.info.Log("event", "blob fetch err", "ref", ref.Ref(), "error", err.Error())
						return
					}

					if newBr.Ref() != ref.Ref() {
						proc.wmgr.info.Log("event", "blob fetch err", "actualRef", newBr.Ref(), "expectedRef", ref.Ref(), "error", "ref did not match expected ref")
						return
					}
				}(w.Ref)
			}
		}
	}

	// shut up if you don't have anything meaningful to add
	if len(mOut) == 0 {
		return nil
	}

	err := proc.out.Pour(ctx, mOut)
	return errors.Wrap(err, "error responding to wants")
}

type WantMsg []want

func (msg *WantMsg) UnmarshalJSON(data []byte) error {
	var wantsMap map[string]int64
	err := json.Unmarshal(data, &wantsMap)
	if err != nil {
		return errors.Wrap(err, "WantMsg: error parsing into map")
	}

	var wants []want
	for ref, dist := range wantsMap {
		ref, err := sbot.ParseRef(ref)
		if err != nil {
			return errors.Wrap(err, "error parsing blob reference")
		}
		br, ok := ref.(*sbot.BlobRef)
		if !ok {
			return errors.Errorf("expected *sbot.BlobRef but got %T", ref)
		}
		wants = append(wants, want{
			Ref:  br,
			Dist: dist,
		})
	}
	*msg = wants
	fmt.Printf("wants debug: %v\n", wants)
	return nil
}
