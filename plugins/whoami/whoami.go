package whoami

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/sbot"
)

var (
	_      sbot.Plugin = plugin{} // compile-time type check
	method             = muxrpc.Method{"whoami"}
	log    logging.Interface
)

func checkAndLog(err error) {
	if err != nil {
		if err := logging.LogPanicWithStack(log, "checkAndLog", err); err != nil {
			panic(err)
		}
	}
}

func init() {
	logging.SetupLogging(nil)
	log = logging.Logger("whoami")
}

func New(n sbot.Repo) sbot.Plugin {
	return plugin{handler{I: n.KeyPair().Id}}
}

type plugin struct {
	h handler
}

func (plugin) Name() string { return "whoami" }

func (plugin) Method() muxrpc.Method {
	return method
}

func (wami plugin) Handler() muxrpc.Handler {
	return wami.h
}

func (plugin) WrapEndpoint(edp muxrpc.Endpoint) interface{} {
	return endpoint{edp}
}

type handler struct {
	I sbot.FeedRef
}

func (handler) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {
	//srv := edp.(muxrpc.Server)
	//log.Log("event", "onConnect", "handler", "whoami", "addr", srv.Remote())
}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request) {
	log.Log("event", "onCall", "handler", "connect", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}
	type ret struct {
		ID string `json:"id"`
	}
	err := req.Return(ctx, ret{h.I.Ref()})
	checkAndLog(err)
}

type endpoint struct {
	edp muxrpc.Endpoint
}

func (edp endpoint) WhoAmI(ctx context.Context) (sbot.FeedRef, error) {
	type respType struct {
		ID sbot.FeedRef `json:"id"`
	}

	var tResp respType

	resp, err := edp.edp.Async(ctx, tResp, method)
	if err != nil {
		return sbot.FeedRef{}, errors.Wrap(err, "error making async call")
	}

	return resp.(respType).ID, nil
}
