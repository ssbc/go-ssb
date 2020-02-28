package processing

import (
	"context"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

type mockMessageProcessor struct {
	procMsgFunc func(context.Context, ssb.Message, margaret.Seq) error
	closeFunc   func(context.Context) error
}

func (mmp mockMessageProcessor) ProcessMessage(ctx context.Context, msg ssb.Message, seq margaret.Seq) error {
	return mmp.procMsgFunc(ctx, msg, seq)
}

func (mmp mockMessageProcessor) Close(ctx context.Context) error {
	return mmp.closeFunc(ctx)
}

type mockMultilog struct {
	getFunc    func(addr librarian.Addr) (margaret.Log, error)
	listFunc   func() ([]librarian.Addr, error)
	deleteFunc func(addr librarian.Addr) error
	closeFunc  func() error
}

func (mml mockMultilog) Get(addr librarian.Addr) (margaret.Log, error) {
	return mml.getFunc(addr)
}

func (mml mockMultilog) List() ([]librarian.Addr, error) {
	return mml.listFunc()
}

func (mml mockMultilog) Delete(addr librarian.Addr) error {
	return mml.deleteFunc(addr)
}

func (mml mockMultilog) Close() error {
	return mml.closeFunc()
}

type mockLog struct {
	seqFunc    func() luigi.Observable
	getFunc    func(margaret.Seq) (interface{}, error)
	appendFunc func(interface{}) (margaret.Seq, error)
}

func (ml mockLog) Seq() luigi.Observable {
	return ml.seqFunc()
}

func (ml mockLog) Get(seq margaret.Seq) (interface{}, error) {
	return ml.getFunc(seq)
}

func (ml mockLog) Query(...margaret.QuerySpec) (luigi.Source, error) {
	panic("not implemented: mockLog.Query")
}

func (ml mockLog) Append(v interface{}) (margaret.Seq, error) {
	return ml.appendFunc(v)
}
