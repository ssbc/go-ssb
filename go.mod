module go.cryptoscope.co/ssb

require (
	github.com/VividCortex/gohistogram v1.0.0 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973 // indirect
	github.com/catherinejones/testdiff v0.0.0-20180525195050-ae148f75f077
	github.com/cryptix/go v1.3.2
	github.com/dgraph-io/badger v1.5.3
	github.com/go-kit/kit v0.7.0
	github.com/gogo/protobuf v1.1.1 // indirect
	github.com/golang/protobuf v1.2.0 // indirect
	github.com/hashicorp/go-multierror v1.0.0
	github.com/keks/nocomment v0.0.0-20180617194237-60ca4c966469
	github.com/kylelemons/godebug v0.0.0-20170820004349-d65d576e9348
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mitchellh/mapstructure v0.0.0-20180715050151-f15292f7a699
	github.com/pkg/errors v0.8.0
	github.com/prometheus/client_golang v0.9.1
	github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910 // indirect
	github.com/prometheus/common v0.0.0-20181020173914-7e9e6cabbd39 // indirect
	github.com/prometheus/procfs v0.0.0-20181005140218-185b4288413d // indirect
	github.com/sergi/go-diff v1.0.0 // indirect
	github.com/shurcooL/go-goon v0.0.0-20170922171312-37c2f522c041
	github.com/stretchr/testify v1.2.2
	go.cryptoscope.co/librarian v0.1.0
	go.cryptoscope.co/luigi v0.3.0
	go.cryptoscope.co/margaret v0.0.7
	go.cryptoscope.co/muxrpc v1.3.0-beta1
	go.cryptoscope.co/netwrap v0.0.0-20180427130219-dae5b5bc35c3
	go.cryptoscope.co/secretstream v1.1.0
	golang.org/x/crypto v0.0.0-20180808211826-de0752318171
	golang.org/x/text v0.3.0
	gonum.org/v1/gonum v0.0.0-20180827050814-95e9db9a70fd
	gonum.org/v1/netlib v0.0.0-20180816165226-ebcc3d2662d3 // indirect
	gopkg.in/urfave/cli.v2 v2.0.0-20180128182452-d3ae77c26ac8
)

replace go.cryptoscope.co/muxrpc => /home/cryptix/go-muxrpc // memleak branch

replace go.cryptoscope.co/luigi => /home/cryptix/go/src/go.cryptoscope.co/luigi/ // race kludge
