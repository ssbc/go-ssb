package repo

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"go.cryptoscope.co/secretstream/secrethandshake"
	"go.cryptoscope.co/ssb"
)

func OpenKeyPair(r Interface) (*ssb.KeyPair, error) {
	secPath := r.GetPath("secret")
	keyPair, err := ssb.LoadKeyPair(secPath)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return nil, errors.Wrap(err, "error opening key pair")
		}

		// generate new keypair
		kp, err := secrethandshake.GenEdKeyPair(nil)
		if err != nil {
			return nil, errors.Wrap(err, "error building key pair")
		}

		keyPair = &ssb.KeyPair{
			Id:   &ssb.FeedRef{ID: kp.Public[:], Algo: "ed25519"},
			Pair: *kp,
		}

		// TODO:
		// keyFile, err := os.Create(secPath)
		// if err != nil {
		// 	return nil, errors.Wrap(err, "error creating secret file")
		// }
		// if err:=ssb.SaveKeyPair(keyFile);err != nil {
		// 	return nil, errors.Wrap(err, "error saving secret file")
		// }
		fmt.Println("warning: save new keypair!")
	}

	return keyPair, nil
}
