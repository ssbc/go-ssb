package imap

import (
	"log"

	"github.com/emersion/go-imap/backend"
	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
)

type User struct {
	pw     []byte
	feed   *ssb.FeedRef
	mboxes namedMboxes
}

type namedMboxes map[string]*Mailbox

func (u *User) Username() string {
	return u.feed.Ref()
}

func (u *User) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	mboxlst := make([]backend.Mailbox, len(u.mboxes))
	for _, mb := range u.mboxes {
		if subscribed && !mb.Subscribed {
			continue
		}
		mboxlst = append(mboxlst, mb)
	}
	return mboxlst, nil
}

func (u *User) GetMailbox(name string) (backend.Mailbox, error) {
	mbox, ok := u.mboxes[name]
	if !ok {
		return nil, errors.Errorf("no such mbox: %s", name)
	}

	return mbox, nil
}

func (u *User) CreateMailbox(name string) error {
	return errors.Errorf("not implemented: CreateMailbox: %s", name)
}

func (u *User) DeleteMailbox(name string) error {
	panic("not implemented - DeleteMailbox")
}

func (u *User) RenameMailbox(existingName string, newName string) error {
	panic("not implemented - RenameMailbox")
}

func (u *User) Logout() error {
	log.Printf("user: logout %s", u.feed.Ref())
	return nil
}
