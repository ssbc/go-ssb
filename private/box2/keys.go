package box2

import (
	"encoding/binary"
	"crypto/sha256"

	"golang.org/x/crypto/hkdf"
)

var (
	infoReadKey Info
	infoHeaderKey Info
	infoBodyKey Info
)

func init() {
	infoReadKey   = Info("keytype:readkey")
	infoHeaderKey = Info("keytype:headerkey")
	infoBodyKey = Info("keytype:bodykey")
}

type Info []byte

func (info Info) Len() int {
	return 2+len(info)
}

type Infos []Info

func (is Infos) Len() int {
	var l int

	for _, info := range is {
		l += info.Len()
	}

	return l
}

type Key []byte

type MessageKey Key

type ReadKey Key

type HeaderKey Key

type BodyKey Key

func (k Key) Derive(buf []byte, infos Infos, outLen int) (Key, error) {
	// if buffer is too short to hold everything, allocate
	if needed := infos.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	var (
		used int
	)
	
	// length-prefix and concatenate info elements
	for _, info := range infos {
		binary.LittleEndian.PutUint16(buf[used:], uint16(len(info)))
		used += 2
		copy(buf[used:], info)
		used += len(info)
	}

	infoBs := buf[:used]
	buf = buf[used:]

	// initialize and perform key derivation
	r := hkdf.New(sha256.New, []byte(k), nil, infoBs)
	out := buf[:outLen]
	_, err := r.Read(out)

	return Key(out), err
}

func (mk MessageKey) DeriveReadKey(buf []byte, infos Infos, outLen int) (ReadKey, error) {
	if needed := infos.Len() + infoReadKey.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	infos = append(infos, infoReadKey)

	k, err := Key(mk).Derive(buf, infos, outLen)
	return ReadKey(k), err
}

func (rk ReadKey) DeriveHeaderKey(buf []byte, infos Infos, outLen int) (HeaderKey, error) {
	if needed := infos.Len() + infoHeaderKey.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	infos = append(infos, infoHeaderKey)

	k, err := Key(rk).Derive(buf, infos, outLen)
	return HeaderKey(k), err
}

func (rk ReadKey) DeriveBodyKey(buf []byte, infos Infos, outLen int) (BodyKey, error) {
	if needed := infos.Len() + infoBodyKey.Len() + outLen; len(buf) < needed {
		buf = make([]byte, needed)
	}

	infos = append(infos, infoBodyKey)

	k, err := Key(rk).Derive(buf, infos, outLen)
	return BodyKey(k), err
}
