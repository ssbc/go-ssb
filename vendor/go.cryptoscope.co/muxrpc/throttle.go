package muxrpc

import (
	"context"
	"fmt"
	"time"
)

type HandlerWrapper func(Handler) Handler

func ApplyHandlerWrappers(h Handler, hws ...HandlerWrapper) Handler {
	for _, hw := range hws {
		h = hw(h)
	}

	return h
}

func Timeout(tout time.Duration) func(Handler) Handler {
	return func(h Handler) Handler {
		return &timeout{
			Handler: h,
			tout:    tout,
		}
	}
}

type timeout struct {
	Handler
	tout time.Duration
}

func (t *timeout) HandleCall(ctx context.Context, req *Request, edp Endpoint) {
	ctx, cancel := context.WithTimeout(ctx, t.tout)
	defer cancel()

	t.Handler.HandleCall(ctx, req, edp)
}

func (t *timeout) HandleConnect(ctx context.Context, edp Endpoint) {
	ctx, cancel := context.WithTimeout(ctx, t.tout)
	defer cancel()

	t.Handler.HandleConnect(ctx, edp)
}

// Throttle limits the number of concurrently processed requests. To prevent starvation, a timeout after which a request is accepted even if the maximum is reached can be specified. If it is 0 no timeout is used.
func Throttle(n int, tout time.Duration) func(Handler) Handler {
	return func(h Handler) Handler {
		return &throttle{
			Handler: h,
			ch:      make(chan struct{}, n),
			tout:    tout,
		}
	}
}

type throttle struct {
	Handler
	ch   chan struct{}
	tout time.Duration
}

func (th *throttle) HandleCall(ctx context.Context, req *Request, edp Endpoint) {
	var timeCh <-chan time.Time

	if th.tout != 0 {
		tr := time.NewTimer(th.tout)
		defer tr.Stop()
		timeCh = tr.C
	}

	select {
	case th.ch <- struct{}{}:
		defer func() {
			<-th.ch
		}()
	case <-timeCh:
	case <-ctx.Done():
		fmt.Println("call cancelled while waiting in throttle")
	}

	th.Handler.HandleCall(ctx, req, edp)
}
