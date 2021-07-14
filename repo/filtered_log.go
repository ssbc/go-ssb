package repo

import (
	"context"
	"fmt"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"
)

type FilterFunc func(refs.Message) bool

func NewFilteredLog(b margaret.Log, fn FilterFunc) margaret.Log {
	return FilteredLog{
		backing: b,
		filter:  fn,
	}
}

type FilteredLog struct {
	backing margaret.Log

	filter FilterFunc
}

func (fl FilteredLog) Seq() luigi.Observable { return fl.backing.Seq() }

// Get retreives the message object by traversing the authors sublog to the root log
func (fl FilteredLog) Get(s margaret.Seq) (interface{}, error) {
	v, err := fl.backing.Get(s)
	if err != nil {
		return nil, fmt.Errorf("publish get: failed to retreive sequence for the root log: %w", err)
	}
	switch tv := v.(type) {
	case error:
		return tv, nil
	case refs.Message:
		if okay := fl.filter(tv); !okay {
			return margaret.ErrNulled, nil
		}
		return tv, nil
	default:
		return nil, fmt.Errorf("unhandled message type: %T", v)
	}
}

func (fl FilteredLog) Query(qry ...margaret.QuerySpec) (luigi.Source, error) {
	src, err := fl.backing.Query(qry...)
	if err != nil {
		return nil, err
	}
	filterdSrc := mfr.SourceFilter(src, func(ctx context.Context, v interface{}) (bool, error) {
		sw := v.(margaret.SeqWrapper)
		iv := sw.Value()

		switch tv := iv.(type) {

		case error:
			if margaret.IsErrNulled(tv) {
				return false, nil
			}
			return false, tv
		case refs.Message:
			if okay := fl.filter(tv); !okay {
				return false, nil
			}
			return true, nil
		default:
			return false, fmt.Errorf("unhandled message type: %T", v)
		}
	})
	return filterdSrc, nil
}

func (fl FilteredLog) Append(val interface{}) (margaret.Seq, error) {
	return nil, fmt.Errorf("FitleredLog is read-only")
}
