package repo

import (
	"log"
	"os"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
)

func OpenKeyPair(r Interface) (*ssb.KeyPair, error) {
	secPath := r.GetPath("secret")
	keyPair, err := ssb.LoadKeyPair(secPath)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return nil, errors.Wrap(err, "repo: error opening key pair")
		}

		keyPair, err = ssb.NewKeyPair(nil)
		if err != nil {
			return nil, errors.Wrap(err, "repo: no keypair but couldn't create one either")
		}

		if err := ssb.SaveKeyPair(keyPair, secPath); err != nil {
			return nil, errors.Wrap(err, "repo: error saving new identity file")
		}
		log.Printf("saved identity %s to %s", keyPair.Id.Ref(), secPath)
	}

	return keyPair, nil
}
