// SPDX-License-Identifier: MIT

package private

import (
	"crypto/rand"
	"log"

	"github.com/pkg/errors"
	"golang.org/x/crypto/nacl/secretbox"

	"go.cryptoscope.co/ssb"
	ssbBox "go.cryptoscope.co/ssb/private/box"
	refs "go.mindeco.de/ssb-refs"
)

// TODO: move to ssb/errors.go
var ErrPrivateMessageDecryptFailed = errors.New("decode pm: decryption failed")

const (
	maxRecps     = 255                         // 1 byte for recipient count
	rcptSboxSize = 32 + 1 + secretbox.Overhead // secretbox secret + rcptCount + overhead
)

var defaultBoxer = ssbBox.NewBoxer(rand.Reader)

func Box(clearMsg []byte, rcpts ...*refs.FeedRef) ([]byte, error) {
	log.Println("deprecated: please move to private/box")
	cipheredMsg, err := defaultBoxer.Encrypt(clearMsg, rcpts...)
	if err != nil {
		return nil, err
	}
	return append([]byte("box1:"), cipheredMsg...), nil
}

func Unbox(recpt *ssb.KeyPair, rawMsg []byte) ([]byte, error) {
	log.Println("deprecated: please move to private/box")
	// TODO: strip prefix?!
	return defaultBoxer.Decrypt(recpt, rawMsg)
}
