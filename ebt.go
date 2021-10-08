// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	refs "go.mindeco.de/ssb-refs"
)

// Note informs about a feeds length and some control settings
type Note struct {
	Seq int64

	// Replicate (seq==-1) tells the peer that it doesn't want to hear about that feed
	Replicate bool

	// Receive controlls the eager push.
	// a peer might want to know if there are updates but not directly get the messages
	Receive bool
}

// MarshalJSON decodes the sequence number and the bools into a single integer for the network layer
func (s Note) MarshalJSON() ([]byte, error) {
	var i int64
	if !s.Replicate {
		return []byte("-1"), nil
	}
	i = int64(s.Seq)
	if i == -1 { // -1 is margarets way of saying "no msgs in this feed"
		i = 0
	}
	i = i << 1 // times 2 (to make room for the rx bit)
	if s.Receive {
		i |= 0
	} else {
		i |= 1
	}
	return []byte(strconv.FormatInt(i, 10)), nil
}

// UnmarshalJSON decodes the network layer pure integer into the sequence number and the receive/replicate bits
func (s *Note) UnmarshalJSON(input []byte) error {
	i, err := strconv.ParseInt(string(input), 10, 64)
	if err != nil {
		return fmt.Errorf("ebt/note: not a valid integer: %w", err)
	}

	if i < -1 {
		return fmt.Errorf("ebt/note: negative number")
	}

	s.Replicate = i != -1
	if !s.Replicate {
		return nil
	}

	s.Receive = !(i&1 == 1)
	s.Seq = int64(i >> 1)

	return nil
}

// NetworkFrontier can be filtered by feed format by setting Format before unmarshaling JSON into it.
type NetworkFrontier struct {
	// TODO: it's a hack to expose the lock directly.
	// it would be much better to hide the lock and the map and expose the map via functions
	sync.Mutex

	Frontier NetworkFrontierMap
}

// NewNetworkFrontier returns a new, initialized map
func NewNetworkFrontier() *NetworkFrontier {
	return &NetworkFrontier{
		Frontier: make(NetworkFrontierMap),
	}
}

// NetworkFrontierMap represents a set of feeds and their length
// The key is the canonical string representation (feed.Ref())
type NetworkFrontierMap map[string]Note

// MarshalJSON turns the frontier into a JSON object where the field is feed ref and the value is a serialzed note integer.
// It also filters out any feed that is not in the set Format of the frontier.
func (nf *NetworkFrontier) MarshalJSON() ([]byte, error) {
	nf.Mutex.Lock()
	defer nf.Mutex.Unlock()

	var filtered = make(NetworkFrontierMap, len(nf.Frontier))

	for fstr, s := range nf.Frontier {
		// validate
		_, err := refs.ParseFeedRef(fstr)
		if err != nil {
			// just skip invalid feeds
			continue
		}

		filtered[fstr] = s
	}
	return json.Marshal(filtered)
}

// UnmarshalJSON expects a JSON object where each field is a feed reference and the value is a EBT Note value.
// If no format is set/expected for the frontier it filters using classic refs.RefAlgoFeedSSB1.
func (nf *NetworkFrontier) UnmarshalJSON(b []byte) error {
	nf.Mutex.Lock()
	defer nf.Mutex.Unlock()

	// first we decode into this dummy map to ignore invalid feed references
	var dummy map[string]Note
	if err := json.Unmarshal(b, &dummy); err != nil {
		return err
	}

	// now copy the dummy entrys, if they are okay and match the wanted format
	var newMap = make(NetworkFrontierMap, len(dummy))
	for fstr, s := range dummy {
		// validate
		_, err := refs.ParseFeedRef(fstr)
		if err != nil {
			// just skip invalid feeds
			continue
		}

		newMap[fstr] = s
	}

	nf.Frontier = newMap
	return nil
}

// String serializes the frontier into a list of feed references and their length
func (nf *NetworkFrontier) String() string {
	nf.Mutex.Lock()
	defer nf.Mutex.Unlock()

	var sb strings.Builder
	sb.WriteString("## Network Frontier:\n")
	for feed, seq := range nf.Frontier {
		fmt.Fprintf(&sb, "\t%s:%+v\n", feed, seq)
	}
	return sb.String()
}
