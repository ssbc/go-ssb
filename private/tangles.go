package private

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/mutil"
	refs "go.mindeco.de/ssb-refs"
)

func (mgr *Manager) getTangleState(root *refs.MessageRef, tname string) refs.TanglePoint {
	addr := librarian.Addr(append(root.Hash, []byte(tname)...)) // TODO: fill tangle
	thandle, err := mgr.tangles.Get(addr)
	if err != nil {
		return refs.TanglePoint{Root: root, Previous: []*refs.MessageRef{root}}
	}

	prev, err := mgr.getLooseEnds(thandle, tname)
	if err != nil {
		panic(err)
	}

	return refs.TanglePoint{Root: root, Previous: append(prev, root)}
}

func (mgr *Manager) getLooseEnds(l margaret.Log, tname string) (refs.MessageRefs, error) {
	src, err := mutil.Indirect(mgr.receiveLog, l).Query()
	if err != nil {
		return nil, err
	}
	todoCtx := context.TODO()
	var tps []refs.TangledPost
	for {
		src, err := src.Next(todoCtx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			return nil, err
		}

		msg, ok := src.(refs.Message)
		if !ok {
			return nil, fmt.Errorf("not a mesg %T", src)
		}

		// decrypt message
		ctxt := mgr.getCiphertext(msg)
		if ctxt == nil {
			continue
		}

		content, err := mgr.DecryptBox2(ctxt, msg.Author(), msg.Previous())
		if err != nil {
			fmt.Println("not for us?", err)
			continue
		}

		// find tangles
		var p struct {
			Tangles refs.Tangles `json:"tangles"`
		}
		err = json.Unmarshal(content, &p)
		if err != nil {
			return nil, err
		}

		tps = append(tps, refs.TangledPost(tangledPost{MessageRef: msg.Key(), Tangles: p.Tangles}))

	}
	sorter := refs.ByPrevious{Items: tps, TangleName: tname}
	sort.Sort(sorter)

	fmt.Println(len(tps), "message in tangle")
	var refs = make(refs.MessageRefs, len(tps))
	for i, post := range tps {
		fmt.Println("ref:", post.Key().Ref())
		refs[i] = post.Key()
	}

	return refs, nil
}

var boxSuffix = []byte(".box2\"")

func (mgr *Manager) getCiphertext(m refs.Message) []byte {
	content := m.ContentBytes()

	if !bytes.HasSuffix(content, boxSuffix) {
		return nil
	}

	n := base64.StdEncoding.DecodedLen(len(content))
	ctxt := make([]byte, n)
	decn, err := base64.StdEncoding.Decode(ctxt, bytes.TrimSuffix(content, boxSuffix)[1:])
	if err != nil {
		return nil
	}
	return ctxt[:decn]
}

type tangledPost struct {
	*refs.MessageRef

	refs.Tangles
}

func (tm tangledPost) Key() *refs.MessageRef {
	return tm.MessageRef
}

func (tm tangledPost) Tangle(name string) (*refs.MessageRef, refs.MessageRefs) {
	tp, has := tm.Tangles[name]
	if !has {
		return nil, nil
	}

	return tp.Root, tp.Previous
}
