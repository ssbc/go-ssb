package graph

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/multilogs"
)

type publisher struct {
	r *require.Assertions

	key     *ssb.KeyPair
	publish margaret.Log
}

func newPublisher(t *testing.T, root margaret.Log, users multilog.MultiLog) *publisher {
	p := &publisher{}
	p.r = require.New(t)

	var err error
	p.key, err = ssb.NewKeyPair(nil)
	p.r.NoError(err)

	p.publish, err = multilogs.OpenPublishLog(root, users, *p.key)
	p.r.NoError(err)

	return p
}

func (p publisher) follow(ref *ssb.FeedRef) {
	newSeq, err := p.publish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   ref.Ref(),
		"following": true,
	})
	p.r.NoError(err)
	p.r.NotNil(newSeq)
}

func (p publisher) block(ref *ssb.FeedRef) {
	newSeq, err := p.publish.Append(map[string]interface{}{
		"type":     "contact",
		"contact":  ref.Ref(),
		"blocking": true,
	})
	p.r.NoError(err)
	p.r.NotNil(newSeq)
}
