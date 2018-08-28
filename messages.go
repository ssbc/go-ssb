package sbot

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
)

func IsWrongTypeErr(err error) bool {
	_, is := errors.Cause(err).(ErrWrongType)
	return is
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
		return errors.Errorf("contact: no type on message")
	}

	if t != "contact" {
		return ErrWrongType{want: "contact", has: t}
	}

	newC := new(Contact)

	newC.Contact, err = ParseFeedRef(potential["contact"].(string))
	if err != nil {
		return errors.Wrap(err, "contact: map stage failed")
	}

	newC.Following, _ = potential["following"].(bool)
	newC.Blocking, _ = potential["blocking"].(bool)

	*c = *newC
	return nil
}
