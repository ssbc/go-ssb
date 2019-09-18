package blobstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}
		wmgr.l.Lock()
		defer wmgr.l.Unlock()

		n, ok := v.(ssb.BlobStoreNotification)
		if !ok {
			return errors.Errorf("blob change: unhandled notification type: %T", v)
		}
		wmgr.promEvent(n.Op.String(), 1)

		if n.Op == ssb.BlobStoreOpPut {
			if _, ok := wmgr.wants[n.Ref.Ref()]; ok {
				delete(wmgr.wants, n.Ref.Ref())

				wmgr.promGaugeSet("nwants", len(wmgr.wants))
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

func (wmgr *wantManager) AllWants() []ssb.BlobWant {
	wmgr.l.Lock()
	defer wmgr.l.Unlock()
	var bws []ssb.BlobWant
	for ref, dist := range wmgr.wants {
		br, err := ssb.ParseBlobRef(ref)
		if err != nil {
			panic(errors.Wrap(err, "invalid blob ref in want manager"))
		}
		bws = append(bws, ssb.BlobWant{
			Ref:  br,
			Dist: dist,
		})
	}
	return bws
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
	dbg := log.With(wmgr.info, "func", "WantWithDist", "ref", ref.Ref(), "dist", dist)
	dbg = level.Debug(dbg)
	_, err := wmgr.bs.Size(ref)
	if err == nil {
		dbg.Log("available", true)
		return nil
	}

	wmgr.l.Lock()
	defer wmgr.l.Unlock()

	wmgr.wants[ref.Ref()] = dist
	wmgr.promGaugeSet("nwants", len(wmgr.wants))

	// TODO: ctx?? this pours into the broadcast, right?
	err = wmgr.wantSink.Pour(context.TODO(), ssb.BlobWant{ref, dist})
	err = errors.Wrap(err, "error pouring want to broadcast")
	dbg.Log("wanting", true)
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

type wantProc struct {
	l sync.Mutex

	rootCtx context.Context

	info log.Logger

	bs          ssb.BlobStore
	wmgr        *wantManager
	out         luigi.Sink
	remoteWants map[string]int64
	done        func(func())
	edp         muxrpc.Endpoint
}

func (proc *wantProc) init() {
	if remote, err := ssb.GetFeedRefFromAddr(proc.edp.Remote()); err == nil {
		proc.info = log.With(proc.wmgr.info, "remote", remote.Ref()[1:5])
	}
	proc.wmgr.promGauge("proc", 1)

	bsCancel := proc.bs.Changes().Register(luigi.FuncSink(proc.updateFromBlobStore))
	wmCancel := proc.wmgr.Register(luigi.FuncSink(proc.updateWants))

	// _i think_ the extra next func is so that the tests can see the shutdown
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

	proc.wmgr.l.Lock()

	err := proc.out.Pour(proc.rootCtx, proc.wmgr.wants)
	level.Debug(proc.wmgr.info).Log("event", "createWants.Out", "cause", "initial wants", "n", len(proc.wmgr.wants))
	if err != nil {
		level.Error(proc.wmgr.info).Log("event", "wantProc.init/Pour", "err", err.Error())
	}
	proc.wmgr.l.Unlock()
}

// updateFromBlobStore listens for adds and if they are wanted notifies the remote via it's sink
func (proc *wantProc) updateFromBlobStore(ctx context.Context, v interface{}, err error) error {
	dbg := level.Debug(proc.info)
	dbg = log.With(dbg, "event", "blobStoreNotify")
	proc.wmgr.l.Lock()
	defer proc.wmgr.l.Unlock()
	// dbg.Log("cause", "HELLOO", "v", fmt.Sprintf("%#v", v), "err", err)
	if err != nil {
		if luigi.IsEOS(err) {
			return nil
		}
		dbg.Log("cause", "update error", "err", err)
		return errors.Wrap(err, "blobstore broadcast error")
	}

	notif, ok := v.(ssb.BlobStoreNotification)
	if !ok {
		err = errors.Errorf("wantProc: unhandled notification type: %T", v)
		level.Error(proc.info).Log("warning", "invalid type", "err", err)
		return err
	}
	dbg = log.With(dbg, "op", notif.Op.String(), "ref", notif.Ref.Ref())

	proc.wmgr.promEvent(notif.Op.String(), 1)

	// if _, wants := proc.remoteWants[notif.Ref.Ref()]; !wants {
	// 	dbg.Log("ignoring", "does not want")
	// 	return nil
	// }

	sz, err := proc.bs.Size(notif.Ref)
	if err != nil {
		return errors.Wrap(err, "error getting blob size")
	}

	m := map[string]int64{notif.Ref.Ref(): sz}
	err = proc.out.Pour(ctx, m)
	dbg.Log("cause", "has wanted blob")
	return errors.Wrap(err, "errors pouring into sink")

}

//
func (proc *wantProc) updateWants(ctx context.Context, v interface{}, err error) error {
	dbg := level.Debug(proc.info)
	if err != nil {
		if luigi.IsEOS(err) {
			return nil
		}
		dbg.Log("cause", "broadcast error", "err", err)
		return errors.Wrap(err, "wmanager broadcast error")
	}

	w, ok := v.(ssb.BlobWant)
	if !ok {
		err := errors.Errorf("wrong type: %T", v)
		proc.info.Log("error", "wrong type!!!!!!!!!!!!!!!!!!!!!!!!", "err", err)
		return err
	}
	dbg = log.With(dbg, "event", "wantBroadcast", "ref", w.Ref.Ref()[1:6], "dist", w.Dist)

	if w.Dist < 0 {
		for want, dist := range proc.remoteWants {
			if want == w.Ref.Ref() {
				dbg.Log("ignoring", "update want", "openDist", dist)
				return nil
			}
		}
	}

	if sz, err := proc.bs.Size(w.Ref); err == nil {
		dbg.Log("whoop", "has size!", "sz", sz)
		return nil
	}

	dbg.Log("op", "sending want we now want", "wantCount", len(proc.wmgr.wants))
	// TODO: should use rootCtx from sbot?
	newW := WantMsg{w}
	return proc.out.Pour(ctx, newW)
}

func (proc *wantProc) getBlob(ctx context.Context, ref *ssb.BlobRef) error {
	src, err := proc.edp.Source(ctx, &WantMsg{}, muxrpc.Method{"blobs", "get"}, ref.Ref())
	if err != nil {
		return errors.Wrap(err, "blob create source failed")
	}

	r := muxrpc.NewSourceReader(src)
	newBr, err := proc.bs.Put(io.LimitReader(r, 25*1024*1024))
	if err != nil {
		return errors.Wrap(err, "blob data piping failed")
	}

	if newBr.Ref() != ref.Ref() {
		// TODO: make this a type of error (forks)
		proc.bs.Delete(newBr)
		return errors.Errorf("blob inconsitency(or size limit) - actualRef(%s) expectedRef(%s)", newBr.Ref(), ref.Ref())
	}
	return nil
}

func (proc *wantProc) Close() error {
	defer proc.done(nil)
	return errors.Wrap(proc.out.Close(), "error in lower-layer close")
}

func (proc *wantProc) Pour(ctx context.Context, v interface{}) error {
	dbg := level.Debug(proc.info)
	dbg = log.With(dbg, "event", "createWants.In")
	dbg.Log("cause", "received data", "v", fmt.Sprintf("%+v %T", v, v))
	proc.l.Lock()
	defer proc.l.Unlock()

	mIn := v.(*WantMsg)
	mOut := make(map[string]int64)

	for _, w := range *mIn {
		wantDist, wanted := proc.remoteWants[w.Ref.Ref()]
		if w.Dist < 0 {
			if wanted {
				dbg.Log("ignoring", "open want!!", "d", wantDist)
				continue
			}
			s, err := proc.bs.Size(w.Ref)
			if err != nil {
				if err == ErrNoSuchBlob {
					proc.remoteWants[w.Ref.Ref()] = w.Dist
					// mOut[w.Ref.Ref()] = w.Dist - 1
					// proc.wmgr.wantSink.Pour(ctx, ssb.BlobWant{Ref: w.Ref, Dist: w.Dist - 1})

					wErr := proc.wmgr.WantWithDist(w.Ref, w.Dist-1)
					if wErr != nil {
						return errors.Wrap(err, "forwarding want faild")
					}
					continue
				}

				return errors.Wrap(err, "error getting blob size")
			}

			delete(proc.remoteWants, w.Ref.Ref())
			mOut[w.Ref.Ref()] = s
		} else if proc.wmgr.Wants(w.Ref) { // someone else wants it?!
			proc.info.Log("event", "createWants.In", "msg", "peer has blob we want", "ref", w.Ref.Ref())
			go func(ref *ssb.BlobRef) {
				// cryptix: feel like we might need to wrap rootCtx in, too?
				if err := proc.getBlob(ctx, ref); err != nil {
					proc.info.Log("event", "blob fetch err", "ref", ref.Ref(), "error", err.Error())
				}
			}(w.Ref)

		} else {
			dbg.Log("unhandled", w.Dist)
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

type WantMsg []ssb.BlobWant

/* turns a blobwant array into one object ala
{
	ref1:dist1,
	ref2:dist2,
	...
}
*/
func (msg WantMsg) MarshalJSON() ([]byte, error) {
	wantsMap := make(map[*ssb.BlobRef]int64, len(msg))
	for _, want := range msg {
		wantsMap[want.Ref] = want.Dist
	}
	data, err := json.Marshal(wantsMap)
	return data, errors.Wrap(err, "WantMsg: error marshalling map?")
}

func (msg *WantMsg) UnmarshalJSON(data []byte) error {
	var directWants []ssb.BlobWant
	err := json.Unmarshal(data, &directWants)
	if err == nil {
		*msg = directWants
		return nil
	}

	var wantsMap map[string]int64
	err = json.Unmarshal(data, &wantsMap)
	if err != nil {
		return errors.Wrap(err, "WantMsg: error parsing into map")
	}

	var wants []ssb.BlobWant
	for ref, dist := range wantsMap {
		br, err := ssb.ParseBlobRef(ref)
		if err != nil {
			return errors.Wrap(err, "WantMsg: error parsing blob reference")
		}

		wants = append(wants, ssb.BlobWant{
			Ref:  br,
			Dist: dist,
		})
	}
	*msg = wants
	return nil
}
