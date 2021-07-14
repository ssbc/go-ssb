package luigiutils

import (
	"context"

	"go.cryptoscope.co/luigi"
)

// JoinSources can be used to combine multiple sources into one.
// The problem with this is keeping the order as intended,
// ie. you dont want to process a leave before the corresponding join or things will get whacky.
// the better approach might be to process the full log and just filter the messages that are relevant.
func JoinSources(srcs ...luigi.Source) luigi.Source {
	return &joinedSource{srcs}
}

type joinedSource struct {
	srcs []luigi.Source
}

func (js *joinedSource) Next(ctx context.Context) (interface{}, error) {
	var lastErr error

	for _, src := range js.srcs {

		v, err := src.Next(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		// shuffle sources around
		js.srcs = append(js.srcs[1:], js.srcs[0])
		return v, nil
	}

	return nil, lastErr
}
