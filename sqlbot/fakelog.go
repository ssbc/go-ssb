package sqlbot

import (
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type sqllog struct{}

func (sl *sqllog) Seq() luigi.Observable {
	panic("not implemented")
}

func (sl *sqllog) Get(seq margaret.Seq) (interface{}, error) {
	panic("not implemented")
}

func (sl *sqllog) Query(...margaret.QuerySpec) (luigi.Source, error) {
	panic("not implemented")
}

func (sl *sqllog) Append(interface{}) (margaret.Seq, error) {
	panic("not implemented")
}
