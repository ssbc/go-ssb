module go.cryptoscope.co/ssb

go 1.13

require (
	github.com/RoaringBitmap/roaring v0.4.21-0.20190925020156-96f2302555b6
	github.com/VividCortex/gohistogram v1.0.0
	github.com/cryptix/go v1.5.0
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger v2.0.0-rc2+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/go-multierror v1.1.0
	github.com/keks/nocomment v0.0.0-20181007001506-30c6dcb4a472
	github.com/keks/persist v0.0.0-20191006175951-43c124092b8b
	github.com/keks/testops v0.1.0
	github.com/kylelemons/godebug v1.1.0
	github.com/libp2p/go-reuseport v0.0.1
	github.com/machinebox/progress v0.2.0
	github.com/matryer/is v1.3.0 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.2.2
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.3.0
	github.com/rs/cors v1.7.0
	github.com/rs/zerolog v1.22.0 // indirect
	github.com/shurcooL/go-goon v0.0.0-20170922171312-37c2f522c041
	github.com/stretchr/testify v1.6.1
	github.com/ugorji/go/codec v1.1.7
	go.cryptoscope.co/librarian v0.2.1-0.20200604160012-d85e03a70e79
	go.cryptoscope.co/luigi v0.3.6-0.20200131144242-3256b54e72c8
	go.cryptoscope.co/margaret v0.1.7-0.20210504155439-deec5d73710c
	go.cryptoscope.co/muxrpc/v2 v2.0.5
	go.cryptoscope.co/netwrap v0.1.1
	go.cryptoscope.co/secretstream v1.2.2
	go.mindeco.de v1.12.0 // indirect
	go.mindeco.de/ssb-gabbygrove v0.1.7-0.20200618115102-169cb68d2398
	go.mindeco.de/ssb-multiserver v0.1.1
	go.mindeco.de/ssb-refs v0.1.1-0.20210413150817-0208d30b0130
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/exp v0.0.0-20190411193353-0480eff6dd7c // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/text v0.3.3
	gonum.org/v1/gonum v0.0.0-20190904110519-2065cbd6b42a
	gopkg.in/urfave/cli.v2 v2.0.0-20190806201727-b62605953717
	modernc.org/kv v1.0.0
)

// We need our internal/extra25519 since agl pulled his repo recently.
// Issue: https://github.com/cryptoscope/ssb/issues/44
// Ours uses a fork of x/crypto where edwards25519 is not an internal package,
// This seemed like the easiest change to port agl's extra25519 to use x/crypto
// Background: https://github.com/agl/ed25519/issues/27#issuecomment-591073699
// The branch in use: https://github.com/cryptix/golang_x_crypto/tree/non-internal-edwards
replace golang.org/x/crypto => github.com/cryptix/golang_x_crypto v0.0.0-20200924101112-886946aabeb8

replace go.mindeco.de/ssb-refs => /home/cryptix/go-repos/ssb-refs

replace go.mindeco.de/ssb-multiserver => /home/cryptix/go-repos/ssb-multiserver

replace go.mindeco.de/ssb-gabbygrove => /home/cryptix/go-repos/gabbygrove
