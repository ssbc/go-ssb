package blobs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/pkg/errors"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/sbot/blobstore"
)

type createWantsHandler struct {
	bs sbot.BlobStore
	wm sbot.WantManager

	// sources is a map if sources where the responses are read from.
	sources map[string]luigi.Source

	// l protects sources.
	l sync.Mutex
}

// getSource looks if we have a source for that remote and, if not, make a
// source call to get one.
func (h createWantsHandler) getSource(ctx context.Context, edp muxrpc.Endpoint) (luigi.Source, error) {
	ref, err := sbot.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		return nil, errors.Wrapf(err, "error getting remote feed ref from addr %#v", edp.Remote())
	}

	h.l.Lock()
	defer h.l.Unlock()

	src, ok := h.sources[ref.Ref()]
	if ok {
		return src, nil
	}

	src, err = edp.Source(ctx, &blobstore.WantMsg{}, muxrpc.Method{"blobs", "createWants"})
	if err != nil {
		return nil, errors.Wrap(err, "error making source call")
	}

	h.sources[ref.Ref()] = src
	return src, nil
}

func (h createWantsHandler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	_, err := h.getSource(ctx, edp)
	if err != nil {
		log.Log("method", "blobs.createWants", "handler", "onConnect", "getSourceErr", err)
		return
	}
}

func (h createWantsHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	log.Log("event", "onCall", "handler", "createWants", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc

	src, err := h.getSource(ctx, edp)
	if err != nil {
		log.Log("event", "onCall", "handler", "createWants", "getSourceErr", err)
		return
	}

	err = luigi.Pump(ctx, h.wm.CreateWants(ctx, req.Stream, edp), src)
	if err != nil {
		log.Log("event", "onCall", "handler", "createWants", "pumpErr", err)
		return
	}
}

type wantProcessor struct {
	bs    sbot.BlobStore
	wants *sync.Map
	ch    chan map[string]int64
}

func (proc wantProcessor) Pour(ctx context.Context, v interface{}) error {
	mIn := v.(map[string]int64)
	mOut := make(map[string]int64)

	for sRef, _ := range mIn {
		_, ok := proc.wants.Load(sRef)
		if !ok {
			continue
		}

		ref, err := sbot.ParseRef(sRef)
		if err != nil {
			return errors.Wrap(err, "error parsing reference")
		}

		r, err := proc.bs.Get(ref.(*sbot.BlobRef))
		if perr := err.(*os.PathError); perr.Err == syscall.ENOENT {
			continue
		} else if err != nil {
			return errors.Wrap(err, "error getting blob")
		}

		f, ok := r.(*os.File)
		if !ok {
			checkAndLog(errors.Errorf("expected blob reader to be a file but it is a %T", r))
			continue
		}

		fi, err := f.Stat()
		if err != nil {
			checkAndLog(errors.Wrap(err, "error getting stat on blob file"))
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
	Ref  *sbot.BlobRef
	Dist int64
}
