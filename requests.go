package ssb

import "encoding/json"

type WhoamiReply struct {
	ID string `json:"id"`
}

type CreateHistArgs struct {
	//map[keys:false id:@Bqm7bG4qvlnWh3BEBFSj2kDr+     30+mUU3hRgrikE2+xc=.ed25519 seq:20 live:true
	Keys bool   `json:"keys"`
	Live bool   `json:"live"`
	Id   string `json:"id"`
	Seq  int    `json:"seq"`
}

type RawSignedMessage struct {
	json.RawMessage
}
