package ssb

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	refs "go.mindeco.de/ssb-refs"
)

// NetworkFrontier represents a set of feeds and their length
type NetworkFrontier map[*refs.FeedRef]Note

// Note informs about a feeds length and some control settings
type Note struct {
	Seq int64

	// Replicate (seq==-1) tells the peer that it doesn't want to hear about that feed
	Replicate bool

	// Receive controlls the eager push.
	// a peer might want to know if there are updates but not directly get the messages
	Receive bool
}

func (s Note) MarshalJSON() ([]byte, error) {
	var i int64
	if !s.Replicate {
		return []byte("-1"), nil
	}
	i = s.Seq << 1
	if s.Receive {
		i |= 0
	} else {
		i |= 1
	}
	return []byte(strconv.FormatInt(i, 10)), nil
}

func (nf *NetworkFrontier) UnmarshalJSON(b []byte) error {
	var dummy map[string]int64

	if err := json.Unmarshal(b, &dummy); err != nil {
		return err
	}

	var newMap = make(NetworkFrontier, len(dummy))
	for fstr, i := range dummy {
		ref, err := refs.ParseFeedRef(fstr)
		if err != nil {
			return err
		}

		var s Note
		s.Replicate = i != -1
		s.Receive = !(i&1 == 1)
		s.Seq = i >> 1

		newMap[ref] = s
	}

	*nf = newMap
	return nil
}

func (nf NetworkFrontier) String() string {
	var sb strings.Builder
	sb.WriteString("## Network Frontier:\n")
	for feed, seq := range nf {
		fmt.Fprintf(&sb, "\t%s:%+v\n", feed.ShortRef(), seq)
	}
	return sb.String()
}
