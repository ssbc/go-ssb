package message

import (
	"strings"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
)

type WhoamiReply struct {
	ID *ssb.FeedRef `json:"id"`
}

func NewCreateHistArgsFromMap(argMap map[string]interface{}) (*CreateHistArgs, error) {

	// could reflect over qrys fiields but meh - compiler knows better
	var qry CreateHistArgs
	for k, v := range argMap {
		switch k = strings.ToLower(k); k {
		case "live", "keys", "values", "reverse", "asjson":
			b, ok := v.(bool)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a bool for %s", k)
			}
			switch k {
			case "live":
				qry.Live = b
			case "keys":
				qry.Keys = b
			case "values":
				qry.Values = b
			case "reverse":
				qry.Reverse = b
			case "asjson":
				qry.AsJSON = b
			}

		case "type":
			fallthrough
		case "id":
			val, ok := v.(string)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a string for %s", k)
			}
			switch k {
			case "id":
				qry.ID = val
				// TODO:
				// case "type":
				// qry.Type = val
			}
		case "seq", "limit":
			n, ok := v.(float64)
			if !ok {
				return nil, errors.Errorf("ssb/message: not a float64(%T) for %s", v, k)
			}
			switch k {
			case "seq":
				qry.Seq = int64(n)
			case "limit":
				qry.Limit = int64(n)
			}
		}
	}

	if qry.Limit == 0 {
		qry.Limit = -1
	}

	return &qry, nil
}

type CommonArgs struct {
	Keys   bool `json:"keys"`
	Values bool `json:"values"`
	Live   bool `json:"live"`

	// this field is used to tell muxrpc into wich type the messages should be marshaled into.
	// for instance, it could be json.RawMessage or a map or a struct
	// TODO: find a nice way to have a default here
	MarshalType interface{} `json:"-"`
}

type CreateHistArgs struct {
	CommonArgs
	ID  string `json:"id"`
	Seq int64  `json:"seq"`

	// TODO: common or stream args?!
	Limit   int64 `json:"limit"`
	Reverse bool  `json:"reverse"`

	AsJSON bool `json:"asJSON"`
}

type StreamArgs struct {
}

type MessagesByTypeArgs struct {
	CommonArgs
	Type string `json:"type"`
}
