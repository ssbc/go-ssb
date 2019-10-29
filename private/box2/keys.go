package box2

import (
	"go.cryptoscope.co/ssb/keys"
)

var (
	infoReadKey   keys.Info
	infoHeaderKey keys.Info
	infoBodyKey   keys.Info
)

func init() {
	// assignment has to happen in init() because
	// we can't assign []byte in a const/var 🙄
	infoReadKey = keys.Info("keytype:readkey")
	infoHeaderKey = keys.Info("keytype:headerkey")
	infoBodyKey = keys.Info("keytype:bodykey")
}

/*
	Key Derivation scheme

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

type MessageKey keys.Key

func (mk MessageKey) DeriveReadKey(buf []byte, infos keys.Infos, outLen int) (ReadKey, error) {
	if needed := infos.Len() + infoReadKey.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	infos = append(infos, infoReadKey)

	k, err := keys.Key(mk).Derive(buf, infos, outLen)
	return ReadKey(k), err
}

type ReadKey keys.Key

type HeaderKey keys.Key

func (rk ReadKey) DeriveHeaderKey(buf []byte, infos keys.Infos, outLen int) (HeaderKey, error) {
	if needed := infos.Len() + infoHeaderKey.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	infos = append(infos, infoHeaderKey)

	k, err := keys.Key(rk).Derive(buf, infos, outLen)
	return HeaderKey(k), err
}

type BodyKey keys.Key

func (rk ReadKey) DeriveBodyKey(buf []byte, infos keys.Infos, outLen int) (BodyKey, error) {
	if needed := infos.Len() + infoBodyKey.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	infos = append(infos, infoBodyKey)

	k, err := keys.Key(rk).Derive(buf, infos, outLen)
	return BodyKey(k), err
}
