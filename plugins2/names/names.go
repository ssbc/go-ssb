package names

import (
	"context"
	"fmt"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
)

type Plugin struct {
	about AboutStore
}

func (lt Plugin) Name() string            { return "names" }
func (Plugin) Method() muxrpc.Method      { return muxrpc.Method{"names"} }
func (lt Plugin) Handler() muxrpc.Handler { return newNamesHandler(nil, lt.about) }

func newNamesHandler(log logging.Interface, as AboutStore) muxrpc.Handler {
	mux := muxrpc.HandlerMux{}

	if log == nil {
		log = logging.Logger("namesHandler")
	}

	var hs = []muxrpc.NamedHandler{
		{muxrpc.Method{"names", "get"}, hGetAll{
			log: log,
			as:  as,
		}},
		{muxrpc.Method{"names", "getImageFor"}, hImagesFor{
			log: log,
			as:  as,
		}},
		{muxrpc.Method{"names", "getSignifier"}, hGetSignifier{
			log: log,
			as:  as,
		}},
	}
	mux.RegisterAll(hs...)

	return &mux
}

type hGetAll struct {
	as  AboutStore
	log logging.Interface
}

func (hGetAll) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hGetAll) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	h.log.Log("event", "onCall", "handler", "getImagesFor", "args", fmt.Sprintf("%v", req.Args), "method", req.Method)
	defer h.log.Log("event", "onCall", "handler", "getImagesFor-return", "method", req.Method)
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	abouts, err := h.as.All()
	if err != nil {
		checkAndLog(h.log, err)
		return
	}
	err = req.Return(ctx, abouts)
	checkAndLog(h.log, errors.Wrap(err, "error returning all values"))
	return
}
