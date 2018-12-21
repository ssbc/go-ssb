module go.cryptoscope.co/muxrpc

require (
	github.com/cryptix/go v1.3.0
	github.com/go-kit/kit v0.7.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/kr/logfmt v0.0.0-20140226030751-b84e30acd515 // indirect
	github.com/pkg/errors v0.8.0
	github.com/stretchr/testify v1.2.2
	go.cryptoscope.co/luigi v0.0.1
)

replace go.cryptoscope.co/luigi => /home/cryptix/go/src/go.cryptoscope.co/luigi/ // race kludge
