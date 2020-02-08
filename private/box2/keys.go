package box2

import (
	"crypto/sha256"

	"go.cryptoscope.co/ssb/keys"
	"golang.org/x/crypto/hkdf"
)

/*
	Key Derivation scheme

	SharedSecret
	 |
	 +-> SlotKey

	MessageKey (randomly sampled by author)
	 |
	 +-> ReadKey
	 |    |
	 |    +-> HeaderKey
     |    |
     |    +-> BodyKey
	 |
	 +-> ExtensionsKey (TODO)
	      |
		  +-> (TODO: Ratcheting, ...)
*/

var (
	infoSlotKey   keys.Info
	infoReadKey   keys.Info
	infoHeaderKey keys.Info
	infoBodyKey   keys.Info
)

func init() {
	// assignment has to happen in init() because
	// we can't assign []byte in a const/var 🙄
	infoReadKey = []byte("slot_key")
	infoReadKey = []byte("read_key")
	infoHeaderKey = []byte("header_key")
	infoBodyKey = []byte("body_key")
}

func deriveTo(out, key []byte, infos ...[]byte) {
	r := hkdf.Expand(sha256.New, key, encodeList(nil, infos))
	r.Read(out)
}
