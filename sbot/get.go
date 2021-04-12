// SPDX-License-Identifier: MIT

package sbot

import (
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	refs "go.mindeco.de/ssb-refs"
)

func (s Sbot) Get(ref refs.MessageRef) (refs.Message, error) {
	getIdx, ok := s.simpleIndex["get"]
	if !ok {
		return nil, fmt.Errorf("sbot: get index disabled")
	}

	obs, err := getIdx.Get(s.rootCtx, storedrefs.Message(ref))
	if err != nil {
		return nil, fmt.Errorf("sbot/get: failed to get seq val from index: %w", err)
	}

	v, err := obs.Value()
	if err != nil {
		return nil, fmt.Errorf("sbot/get: failed to get current value from obs: %w", err)
	}

	var seq margaret.Seq
	switch tv := v.(type) {
	case margaret.Seq:
		seq = tv
	case int64:
		if tv < 0 {
			return nil, fmt.Errorf("invalid sequence stored in index")
		}
		seq = margaret.BaseSeq(tv)
	default:
		return nil, fmt.Errorf("sbot/get: wrong sequence type in index: %T", v)
	}

	storedV, err := s.ReceiveLog.Get(seq)
	if err != nil {
		return nil, fmt.Errorf("sbot/get: failed to load message: %w", err)
	}

	msg, ok := storedV.(refs.Message)
	if !ok {
		return nil, fmt.Errorf("sbot/get: wrong message type in storeage: %T", storedV)
	}

	return msg, nil
}
