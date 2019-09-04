package status

import (
	"context"
	"log"
	"net"
	"sort"
	"time"

	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb/internal/multiserver"

	"github.com/dustin/go-humanize"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
)

type Plugin struct {
	root margaret.Log
	net  ssb.Network
}

func New(n ssb.Network, root margaret.Log) *Plugin {
	return &Plugin{
		root: root,
		net:  n,
	}
}

func (lt Plugin) Name() string            { return "status" }
func (Plugin) Method() muxrpc.Method      { return muxrpc.Method{"status"} }
func (lt Plugin) Handler() muxrpc.Handler { return lt }

func (g Plugin) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (g Plugin) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	v, err := g.root.Seq().Value()
	if err != nil {
		log.Println("statuserr", err)
		return
	}

	s := status{
		Root: v.(margaret.Seq),
	}
	edps := g.net.GetAllEndpoints()

	sort.Sort(byConnTime(edps))

	for _, es := range edps {
		var ms multiserver.NetAddress
		ms.Ref = es.ID
		if tcpAddr, ok := netwrap.GetAddr(es.Addr, "tcp").(*net.TCPAddr); ok {
			ms.Addr = *tcpAddr
		}
		s.Peers = append(s.Peers, peerStatus{
			Addr:  ms.String(),
			Since: humanize.Time(time.Now().Add(-es.Since)),
		})
	}
	err = req.Return(ctx, s)
	log.Println("statuserr", err)
}

type byConnTime []ssb.EndpointStat

func (bct byConnTime) Len() int {
	return len(bct)
}

func (bct byConnTime) Less(i int, j int) bool {
	return bct[i].Since < bct[j].Since
}

func (bct byConnTime) Swap(i int, j int) {
	bct[i], bct[j] = bct[j], bct[i]
}

type peerStatus struct {
	Addr  string
	Since string
}
type status struct {
	Root  margaret.Seq
	Peers []peerStatus
}
