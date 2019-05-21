package blobs

import (
	"context"
	"os"
	"sync"
	"syscall"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
)

type createWantsHandler struct {
	log logging.Interface
	bs  ssb.BlobStore
	wm  ssb.WantManager

	// sources is a map if sources where the responses are read from.
	sources map[string]luigi.Source

	// l protects sources.
	l sync.Mutex
}

// getSource looks if we have a source for that remote and, if not, make a
// source call to get one.
func (h *createWantsHandler) getSource(ctx context.Context, edp muxrpc.Endpoint) (luigi.Source, error) {
	ref, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		return nil, errors.Wrapf(err, "error getting remote feed ref from addr %#v", edp.Remote())
	}

	h.l.Lock()
	defer h.l.Unlock()

	src, ok := h.sources[ref.Ref()]
	if ok {
		if src != nil {
			return src, nil
		}

		h.log.Log("msg", "got a nil source from the map, ignoring and making new")
	}

	src, err = edp.Source(ctx, &blobstore.WantMsg{}, muxrpc.Method{"blobs", "createWants"})
	if err != nil {
		return nil, errors.Wrap(err, "error making source call")
	}
	if src == nil {
		h.log.Log("msg", "got a nil source edp.Source, returning an error")
		//debug.PrintStack()
		return nil, errors.New("could not make createWants call")
	}

	h.sources[ref.Ref()] = src
	return src, nil
}

func (h *createWantsHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	_, err := h.getSource(ctx, edp)
	if err != nil {
		h.log.Log("method", "blobs.createWants", "handler", "onConnect", "getSourceErr", err)
		return
	}
	<-ctx.Done()
}

func (h *createWantsHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	src, err := h.getSource(ctx, edp)
	if err != nil {
		h.log.Log("event", "onCall", "handler", "createWants", "getSourceErr", err)
		return
	}
	snk := h.wm.CreateWants(ctx, req.Stream, edp)
	err = luigi.Pump(ctx, snk, src)
	if err != nil {
		h.log.Log("event", "onCall", "handler", "createWants", "pumpErr", err)
	}
	snk.Close()
}

type wantProcessor struct {
	bs    ssb.BlobStore
	wants *sync.Map
	ch    chan map[string]int64
	log   logging.Interface
}

func (proc wantProcessor) Pour(ctx context.Context, v interface{}) error {
	mIn := v.(map[string]int64)
	mOut := make(map[string]int64)

	for sRef := range mIn {
		_, ok := proc.wants.Load(sRef)
		if !ok {
			continue
		}

		ref, err := ssb.ParseRef(sRef)
		if err != nil {
			return errors.Wrap(err, "error parsing reference")
		}

		r, err := proc.bs.Get(ref.(*ssb.BlobRef))
		if perr := err.(*os.PathError); perr.Err == syscall.ENOENT {
			continue
		} else if err != nil {
			return errors.Wrap(err, "error getting blob")
		}

		f, ok := r.(*os.File)
		if !ok {
			checkAndLog(proc.log, errors.Errorf("expected blob reader to be a file but it is a %T", r))
			continue
		}

		fi, err := f.Stat()
		if err != nil {
			checkAndLog(proc.log, errors.Wrap(err, "error getting stat on blob file"))
			continue
		}

		mOut[sRef] = fi.Size()
	}

	select {
	case proc.ch <- mOut:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (proc *wantProcessor) Next(ctx context.Context) (interface{}, error) {
	select {
	case m := <-proc.ch:
		return m, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}

}

type want struct {
	Ref  *ssb.BlobRef
	Dist int64
}
