package ssb

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
)

func IsMessageUnusable(err error) bool {
	_, is := errors.Cause(err).(ErrWrongType)
	if is {
		return true
	}
	_, is = errors.Cause(err).(ErrMalfromedMsg)
	return is
}

type ErrMalfromedMsg struct {
	reason string
	m      map[string]interface{}
}

func (emm ErrMalfromedMsg) Error() string {
	s := "ErrMalfromedMsg: " + emm.reason
	if emm.m != nil {
		s += fmt.Sprintf("%+v", emm.m)
	}
	return s
}

type ErrWrongType struct {
	has, want string
}

func (ewt ErrWrongType) Error() string {
	return fmt.Sprintf("ErrWrongType: want: %s has: %s", ewt.want, ewt.has)
}

type Contact struct {
	Contact             *FeedRef
	Following, Blocking bool
}

func (c *Contact) UnmarshalJSON(b []byte) error {
	var priv string
	err := json.Unmarshal(b, &priv)
	if err == nil {
		return ErrWrongType{want: "contact", has: "private.box?"}
	}

	var potential map[string]interface{}
	err = json.Unmarshal(b, &potential)
	if err != nil {
		return errors.Wrap(err, "contact: map stage failed")
	}

	t, ok := potential["type"].(string)
	if !ok {
		return ErrMalfromedMsg{"contact: no type on message", nil}
	}

	if t != "contact" {
		return ErrWrongType{want: "contact", has: t}
	}

	newC := new(Contact)

	contact, ok := potential["contact"].(string)
	if !ok {
		return ErrMalfromedMsg{"contact: no string contact field on type:contact", potential}
	}

	newC.Contact, err = ParseFeedRef(contact)
	if err != nil {
		return errors.Wrap(err, "contact: map stage failed")
	}

	newC.Following, _ = potential["following"].(bool)
	newC.Blocking, _ = potential["blocking"].(bool)

	*c = *newC
	return nil
}
