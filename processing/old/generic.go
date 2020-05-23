package processing

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

type KVPair struct {
	Key, Value []byte
}

func (p KVPair) Encode() []byte {
	w := bytes.NewBuffer(make([]byte, 0, 4+len(p.Key)+len(p.Value)))

	// buffers don't return errors
	p.EncodeTo(w)

	return w.Bytes()
}

func (p KVPair) EncodeTo(w io.Writer) error {
	err := binary.Write(w, binary.BigEndian, uint16(len(p.Key)))
	if err != nil {
		return err
	}

	_, err = w.Write(p.Key)
	if err != nil {
		return err
	}

	err = binary.Write(w, binary.BigEndian, uint16(len(p.Value)))
	if err != nil {
		return err
	}

	_, err = w.Write(p.Value)
	return err
}

type Index struct {
	xtrs []MessageExtractor
	mlog multilog.MultiLog
}

func (idx Index) Process(ctx context.Context, seq margaret.Seq, msg ssb.Message) error {
	var (
		log margaret.Log
		err error
	)

	// for all xtractors
	for _, xtr := range idx.xtrs {

		// for all kvs returned by this extractor
		for _, kv := range xtr(msg) {
			log, err = idx.mlog.Get(librarian.Addr(kv.Encode()))
			if err != nil {
				return err
			}

			_, err = log.Append(seq)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (idx Index) Close() error {
	return idx.mlog.Close()
}

func OpenGeneric(r repo.Interface, name string, xtrs []MessageExtractor) (multilog.MultiLog, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, name, func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		idx := Index{
			xtrs: xtrs,
			mlog: mlog,
		}

		msg, ok := value.(ssb.Message)
		if !ok {
			return fmt.Errorf("unexpected type %T, exptected %T", value, msg)
		}

		return idx.Process(ctx, seq, msg)
	})
}

/*
type ChainedMapFunc func(next luigi.MapFunc) luigi.MapFunc

func (cmf ChainedMapFunc) Then(next ChainedMapFunc) ChainedMapFunc {
	return ChainedMapFunc(func(nextMF luigi.MapFunc) luigi.MapFunc {
		return luigi.MapFunc(func(ctx context.Context, v interface{}) (interface{}, error){
			return cmf(next(nextMF))
		})
	})
}

type GenericExtractor func(v interface{}) (map[string]string, error)

type PluggableExtractor func(next GenericExtractor) GenericExtractor

func (px PluggableExtractor) Then(next PluggableExtractor) PluggableExtractor {
	return PluggableExtractor(func(nextExt GenericExtractor) GenericExtractor {
		return px(next(nextExt))
	})
}

func (px PluggableExtractor) Assert() GenericExtractor {
	return Terminate(px)
}

func OpenGeneric(r repo.Interface, name string, sf luigi.MapFunc) (multilog.MultiLog, *badger.DB, repo.ServeFunc, error) {
	return repo.OpenMultiLog(r, name, func(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
		if nulled, ok := value.(error); ok {
			if margaret.IsErrNulled(nulled) {
				return nil
			}
			return nulled
		}

		m, err := x(value)
		if err != nil {
			// TODO log or handle
			// tending towards logging because the caller probably can't handle it either
			return nil
		}

		var errs []error

		for k, v := range m {
			id, err := EncodeStringTuple(k, v)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			kvLog, err := mlog.Get(id)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			_, err = kvLog.Append(seq)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}

		// TODO use proper multierror thing here
		if len(errs) > 0 {
			var acc string
			for _, e := range errs {
				acc += fmt.Sprintf("\n - %s", e.Error())
			}
			return errors.Errorf("%d errors occurred:%s", len(errs), acc)
		}

		return nil
	})
}

func Plug(pexts ...PluggableExtractor) GenericExtractor {
	var ext = Terminate(pexts[len(pexts)-1])

	for i := len(pexts) - 2; i >= 0; i-- {
		ext = pexts[i](ext)
	}

	return ext
}

func Terminate(pext PluggableExtractor) GenericExtractor {
	return pext(func(v interface{}) (map[string]string, error) {
		m, ok := v.(map[string]string)
		if !ok {
			return nil, fmt.Errorf("expected type %T, got %T", m, v)
		}

		return m, nil
	})
}

func NewStoredMessageRawExtractor() PluggableExtractor {
	return PluggableExtractor(func(next GenericExtractor) GenericExtractor {
		return GenericExtractor(func(v interface{}) (map[string]string, error) {
			msg, ok := v.(message.StoredMessage)
			if !ok {
				fmt.Printf("expected type %T, got %T\n", msg, v)
				// TODO log?
				return nil, nil
			}

			return next(msg.Raw)
		})
	})
}

func NewJSONDecodeToContentExtractor() PluggableExtractor {
	return PluggableExtractor(func(next GenericExtractor) GenericExtractor {
		return GenericExtractor(func(v interface{}) (map[string]string, error) {
			var m map[string]interface{}

			err := json.Unmarshal(v.([]byte), &m)
			if err != nil {
				return nil, err
			}

			return next(m)
		})
	})
}

func NewTraverseExtractor(path []string) PluggableExtractor {
	return PluggableExtractor(func(next GenericExtractor) GenericExtractor {
		return GenericExtractor(func(v interface{}) (map[string]string, error) {
			// don't operate on path directly, or else it will only work
			// for the first call.
			var remaining = path

			for len(remaining) > 0 {
				m, ok := v.(map[string]interface{})
				if !ok {
					// TODO log?
					return nil, nil
				}

				v, ok = m[remaining[0]]
				remaining = remaining[1:]
				if !ok {
					// TODO log?
					return nil, nil
				}
			}

			return next(v)
		})
	})
}

func StringsExtractor(max int) PluggableExtractor {
	return PluggableExtractor(func(next GenericExtractor) GenericExtractor {
		return GenericExtractor(func(v interface{}) (map[string]string, error) {
			m, ok := v.(map[string]interface{})
			if !ok {
				// TODO log?
				return nil, nil
			}

			out := make(map[string]string)
			for k, v := range m {
				str, ok := v.(string)
				if !ok || len(str) > max {
					continue
				}

				out[k] = str
			}

			return out, nil
		})
	})
}

// EncodeStringTuple encodes a pair of strings to bytes by length-prefixing and then concatenating them. Returns an error if either input string is longer than 255 bytes.
func EncodeStringTuple(str1, str2 string) (librarian.Addr, error) {
	var (
		bs1, bs2 = []byte(str1), []byte(str2)
		l1, l2   = len(bs1), len(bs2)
		buf      = make([]byte, l1+l2+2)
	)

	fmtStr := "could not encode string tuple: %s string too long (%d>255)"

	if l1 > 255 {
		return "", errors.Errorf(fmtStr, "first", l1)
	}

	if l2 > 255 {
		return "", errors.Errorf(fmtStr, "second", l2)
	}

	buf[0] = byte(l1)
	buf[l1+1] = byte(l2)

	copy(buf[1:], bs1)
	copy(buf[2+l1:], bs2)

	return librarian.Addr(buf), nil
}
*/
