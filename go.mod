module go.cryptoscope.co/ssb

go 1.13

require (
	github.com/RoaringBitmap/roaring v0.6.1
	github.com/VividCortex/gohistogram v1.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger/v3 v3.2011.1
	github.com/dgraph-io/sroar v0.0.0-20210524170324-9b164cbe6e02
	github.com/dustin/go-humanize v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/go-multierror v1.1.1
	github.com/keks/persist v0.0.0-20210520094901-9bdd97c1fad2
	github.com/keks/testops v0.1.0
	github.com/kr/pretty v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0
	github.com/libp2p/go-reuseport v0.0.1
	github.com/machinebox/progress v0.2.0
	github.com/matryer/is v1.3.0 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.2.2
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.3.0
	github.com/rs/cors v1.7.0
	github.com/shurcooL/go v0.0.0-20200502201357-93f07166e636 // indirect
	github.com/shurcooL/go-goon v0.0.0-20170922171312-37c2f522c041
	github.com/stretchr/testify v1.7.0
	github.com/ugorji/go/codec v1.2.6
	go.cryptoscope.co/luigi v0.3.6-0.20200131144242-3256b54e72c8
	go.cryptoscope.co/margaret v0.2.1-0.20210604193815-c622a8ba2526
	go.cryptoscope.co/muxrpc/v2 v2.0.5
	go.cryptoscope.co/netwrap v0.1.1
	go.cryptoscope.co/nocomment v0.0.0-20210520094614-fb744e81f810
	go.cryptoscope.co/secretstream v1.2.2
	go.mindeco.de v1.12.0
	go.mindeco.de/ssb-gabbygrove v0.2.0
	go.mindeco.de/ssb-multiserver v0.1.2
	go.mindeco.de/ssb-refs v0.3.1
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/exp v0.0.0-20190411193353-0480eff6dd7c // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/text v0.3.6
	gonum.org/v1/gonum v0.0.0-20190904110519-2065cbd6b42a
	gopkg.in/urfave/cli.v2 v2.0.0-20190806201727-b62605953717
	modernc.org/kv v1.0.3
)

// We need our internal/extra25519 since agl pulled his repo recently.
// Issue: https://github.com/cryptoscope/ssb/issues/44
// Ours uses a fork of x/crypto where edwards25519 is not an internal package,
// This seemed like the easiest change to port agl's extra25519 to use x/crypto
// Background: https://github.com/agl/ed25519/issues/27#issuecomment-591073699
// The branch in use: https://github.com/cryptix/golang_x_crypto/tree/non-internal-edwards
replace golang.org/x/crypto => github.com/cryptix/golang_x_crypto v0.0.0-20200924101112-886946aabeb8
