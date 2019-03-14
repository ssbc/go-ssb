package imap

import (
	"encoding/hex"

	"github.com/cryptix/go/logging"
	"github.com/emersion/go-imap/backend"
	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
	"golang.org/x/crypto/bcrypt"
)

type Backend struct {
	knownUsers map[string]*User
}

func NewBackend() *Backend {
	pw, err := hex.DecodeString("2432612431302436634243424a573331474f6261545550564957462f756962393438416e5a6730474a71643734755478544543314e31595333455a43")
	logging.CheckFatal(err)

	feed, err := ssb.ParseFeedRef("@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519")
	logging.CheckFatal(err)
	be := &Backend{
		knownUsers: make(map[string]*User),
	}
	be.knownUsers["cryptix"] = &User{
		pw:   pw,
		feed: feed,
		mboxes: namedMboxes{
			"INBOX":   &Mailbox{name: "INBOX"},
			"public":  &Mailbox{name: "public"},
			"friends": &Mailbox{name: "friends"},
		},
	}
	return be
}

// Messages: []*Message{
// 	{
// 		Uid:   6,
// 		Date:  time.Now(),
// 		Flags: []string{"\\Seen"},
// 		Size:  uint32(len(body)),
// 		Body:  []byte(body),
// 	},
// },

func (be *Backend) Login(username string, password string) (backend.User, error) {
	u, ok := be.knownUsers[username]
	if !ok {
		return nil, errors.Errorf("no such user")
	}

	if err := bcrypt.CompareHashAndPassword(u.pw, []byte(password)); err != nil {
		return nil, errors.Wrapf(err, "invallid login for %s", username)
	}

	return u, nil
}
