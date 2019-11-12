module go.cryptoscope.co/ssb

go 1.13

require (
	github.com/AndreasBriese/bbloom v0.0.0-20190825152654-46b345b51c96 // indirect
	github.com/VividCortex/gohistogram v1.0.0 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412
	github.com/catherinejones/testdiff v0.0.0-20180525195050-ae148f75f077
	github.com/cryptix/go v1.5.0
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger v2.0.0-rc2+incompatible
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/go-kit/kit v0.9.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/keks/nocomment v0.0.0-20181007001506-30c6dcb4a472
	github.com/keks/testops v0.1.0
	github.com/kylelemons/godebug v1.1.0
	github.com/libp2p/go-reuseport v0.0.1
	github.com/mattn/go-sqlite3 v1.11.0
	github.com/maxbrunsfeld/counterfeiter/v6 v6.2.2
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/sergi/go-diff v1.0.0 // indirect
	github.com/shurcooL/go-goon v0.0.0-20170922171312-37c2f522c041
	github.com/stretchr/testify v1.4.0
	github.com/ugorji/go/codec v1.1.7
	go.cryptoscope.co/librarian v0.2.0
	go.cryptoscope.co/luigi v0.3.5-0.20190924074117-8ca146aad481
	go.cryptoscope.co/margaret v0.0.13-0.20191112171912-4f8f49bca4d5
	go.cryptoscope.co/muxrpc v1.5.0
	go.cryptoscope.co/netwrap v0.1.0
	go.cryptoscope.co/secretstream v1.2.0
	go.mindeco.de/ssb-gabbygrove v0.1.5
	golang.org/x/crypto v0.0.0-20190911031432-227b76d455e7
	golang.org/x/sys v0.0.0-20190911201528-7ad0cfa0b7b5 // indirect
	golang.org/x/text v0.3.2
	gonum.org/v1/gonum v0.0.0-20190904110519-2065cbd6b42a
	gopkg.in/urfave/cli.v2 v2.0.0-20190806201727-b62605953717
	modernc.org/kv v1.0.0
)

replace github.com/keks/persist => github.com/cryptix/keks_persist v0.0.0-20190924155924-a51e5e7eb3e6
