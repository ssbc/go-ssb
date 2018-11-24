package blobstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
)

func NewWantManager(log logging.Interface, bs ssb.BlobStore, opts ...interface{}) ssb.WantManager {
	wmgr := &wantManager{
		bs:    bs,
		wants: make(map[string]int64),
		info:  log,
	}

	for i, o := range opts {
		switch v := o.(type) {
		case *prometheus.Gauge:
			wmgr.gauge = v
		case *prometheus.Counter:
			wmgr.evtCtr = v
		default:
			log.Log("warning", "unhandled option", "i", i, "type", fmt.Sprintf("%T", o))
		}
	}

	wmgr.promGaugeSet("proc", 0)

	wmgr.wantSink, wmgr.Broadcast = luigi.NewBroadcast()

	bs.Changes().Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		wmgr.l.Lock()
		defer wmgr.l.Unlock()

		n, ok := v.(ssb.BlobStoreNotification)
		if ok {
			wmgr.promEvent(n.Op.String(), 1)
			switch n.Op {
			case ssb.BlobStoreOpPut:
				if _, ok := wmgr.wants[n.Ref.Ref()]; ok {
					delete(wmgr.wants, n.Ref.Ref())

					wmgr.promGaugeSet("nwants", len(wmgr.wants))
				}
			default:
				log.Log("evnt", "warn/debug", "msg", "unhandled blobStore change", "notify", n)
			}
		}

		return nil
	}))

	return wmgr
}

type wantManager struct {
	luigi.Broadcast

	bs ssb.BlobStore

	wants    map[string]int64
	wantSink luigi.Sink

	l sync.Mutex

	info   logging.Interface
	evtCtr *prometheus.Counter
	gauge  *prometheus.Gauge
}

func (wmgr *wantManager) promEvent(name string, n float64) {
	if wmgr.evtCtr != nil {
		wmgr.evtCtr.With("event", name).Add(n)
	}
}

func (wmgr *wantManager) promGauge(name string, n float64) {
	if wmgr.gauge != nil {
		wmgr.gauge.With("part", name).Add(n)
	}
}
func (wmgr *wantManager) promGaugeSet(name string, n int) {
	if wmgr.gauge != nil {
		wmgr.gauge.With("part", name).Set(float64(n))
	}
}

func (wmgr *wantManager) Wants(ref *ssb.BlobRef) bool {
	wmgr.l.Lock()
	defer wmgr.l.Unlock()

	_, ok := wmgr.wants[ref.Ref()]
	return ok
}

func (wmgr *wantManager) Want(ref *ssb.BlobRef) error {
	return wmgr.WantWithDist(ref, -1)
}

func (wmgr *wantManager) WantWithDist(ref *ssb.BlobRef, dist int64) error {
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
	wmgr.promGaugeSet("nwants", len(wmgr.wants))

	// TODO: ctx?? this pours into the broadcast, right?
	err = wmgr.wantSink.Pour(context.TODO(), want{ref, dist})
	err = errors.Wrap(err, "error pouring want to broadcast")
	return err
}

func (wmgr *wantManager) CreateWants(ctx context.Context, sink luigi.Sink, edp muxrpc.Endpoint) luigi.Sink {
	proc := &wantProc{
		rootCtx:     ctx,
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
	Ref *ssb.BlobRef

	// if Dist is negative, it is the hop count to the original wanter.
	// if it is positive, it is the size of the blob.
	Dist int64
}

type wantProc struct {
	l sync.Mutex

	rootCtx context.Context

	bs          ssb.BlobStore
	wmgr        *wantManager
	out         luigi.Sink
	remoteWants map[string]int64
	done        func(func())
	edp         muxrpc.Endpoint
}

func (proc *wantProc) init() {

	proc.wmgr.promGauge("proc", 1)

	bsCancel := proc.bs.Changes().Register(
		luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			proc.l.Lock()
			defer proc.l.Unlock()

			notif := v.(ssb.BlobStoreNotification)
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
		proc.wmgr.promGauge("proc", -1)
		bsCancel()
		wmCancel()
		if oldDone != nil {
			oldDone(nil)
		}
		if next != nil {
			next()
		}
	}

	err := proc.out.Pour(proc.rootCtx, proc.wmgr.wants)
	// proc.wmgr.info.Log("event", "createWants.Out", "cause", "initial wants")
	if err != nil {
		proc.wmgr.info.Log("event", "wantProc.init/Pour", "err", err.Error())
	}
}

func (proc *wantProc) Close() error {
	defer proc.done(nil)
	return errors.Wrap(proc.out.Close(), "error in lower-layer close")
}

func (proc *wantProc) Pour(ctx context.Context, v interface{}) error {
	// proc.wmgr.info.Log("event", "createWants.In", "cause", "received data")
	proc.l.Lock()
	defer proc.l.Unlock()

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
				go func(ref *ssb.BlobRef) {
					// cryptix: feel like we might need to wrap rootCtx in, too?
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

	// cryptix: feel like we might need to wrap rootCtx in, too?
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
		ref, err := ssb.ParseRef(ref)
		if err != nil {
			return errors.Wrap(err, "error parsing blob reference")
		}
		br, ok := ref.(*ssb.BlobRef)
		if !ok {
			return errors.Errorf("expected *ssb.BlobRef but got %T", ref)
		}
		wants = append(wants, want{
			Ref:  br,
			Dist: dist,
		})
	}
	*msg = wants
	return nil
}
