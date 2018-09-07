package repo

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"go.cryptoscope.co/sbot"
	"go.cryptoscope.co/secretstream/secrethandshake"
)

func OpenKeyPair(r Interface) (*sbot.KeyPair, error) {
	secPath := r.GetPath("secret")
	keyPair, err := sbot.LoadKeyPair(secPath)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return nil, errors.Wrap(err, "error opening key pair")
		}

		// generate new keypair
		kp, err := secrethandshake.GenEdKeyPair(nil)
		if err != nil {
			return nil, errors.Wrap(err, "error building key pair")
		}

		keyPair = &sbot.KeyPair{
			Id:   &sbot.FeedRef{ID: kp.Public[:], Algo: "ed25519"},
			Pair: *kp,
		}

		// TODO:
		// keyFile, err := os.Create(secPath)
		// if err != nil {
		// 	return nil, errors.Wrap(err, "error creating secret file")
		// }
		// if err:=sbot.SaveKeyPair(keyFile);err != nil {
		// 	return nil, errors.Wrap(err, "error saving secret file")
		// }
		fmt.Println("warning: save new keypair!")
	}

	return keyPair, nil
}
