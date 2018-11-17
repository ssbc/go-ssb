package muxrpc

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

type HandlerMux struct {
	handlers map[string]Handler
}

func (hm *HandlerMux) HandleCall(ctx context.Context, req *Request, edp Endpoint) {
	for i := len(req.Method); i > 0; i-- {
		m := req.Method[:i]
		h, ok := hm.handlers[m.String()]
		if ok {
			h.HandleCall(ctx, req, edp)
			return
		}
	}

	req.Stream.CloseWithError(errors.Errorf("no such command: %v", req.Method))
}

func (hm *HandlerMux) HandleConnect(ctx context.Context, edp Endpoint) {
	var wg sync.WaitGroup
	wg.Add(len(hm.handlers))

	for _, h := range hm.handlers {
		go func(h Handler) {
			defer wg.Done()

			h.HandleConnect(ctx, edp)
		}(h)
	}

	wg.Wait()
}

func (hm *HandlerMux) Register(m Method, h Handler) {
	if hm.handlers == nil {
		hm.handlers = make(map[string]Handler)
	}

	hm.handlers[m.String()] = h
}
