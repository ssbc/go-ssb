package private

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/private/box2"

	"go.cryptoscope.co/ssb/keys"
	refs "go.mindeco.de/ssb-refs"
)

type groupInit struct {
	Type string `json:"type"`
	Name string `json:"name"`

	Tangles struct {
		Group struct {
			Root     *refs.Ref   `json:"root"`
			Previous []*refs.Ref `json:"previous"`
		} `json:"group"`
	} `json:"tangles"`
}

func (mgr *Manager) Init(name string) (*refs.MessageRef, error) {
	// roll new key
	var r keys.Recipient
	r.Scheme = keys.SchemeLargeSymmetricGroup
	r.Key = make([]byte, 32) // TODO: key size const
	_, err := rand.Read(r.Key)
	if err != nil {
		return nil, err
	}

	var gi groupInit
	gi.Type = "group/init"
	gi.Name = name

	// encrypt the group/init message

	jsonContent, err := json.Marshal(gi)
	if err != nil {
		return nil, err
	}

	bxr := box2.NewBoxer(mgr.rand)

	// TODO: protect against race of changing previous
	// mgr.publog.Lock()

	// get previous
	currSeqV, err := mgr.publog.Seq().Value()
	if err != nil {
		return nil, err
	}
	currSeq := currSeqV.(margaret.Seq)
	msgV, err := mgr.publog.Get(currSeq)
	if err != nil {
		return nil, err
	}
	msg := msgV.(refs.Message)
	prev := msg.Key()
	fmt.Println(prev.ShortRef())
	// now create the ciphertext
	ciphertext, err := bxr.Encrypt(jsonContent, mgr.author, prev, keys.Recipients{r})
	if err != nil {
		return nil, err
	}

	// TODO: format check for gabbygrove
	content := base64.StdEncoding.EncodeToString(ciphertext) + ".box2"

	initPublish, err := mgr.publog.Publish(content)
	if err != nil {
		return nil, err
	}

	err = mgr.keymgr.AddKey(r.Scheme, initPublish.Hash, r.Key)
	if err != nil {
		return nil, err
	}
	// my keys
	err = mgr.keymgr.AddKey(r.Scheme, sortAndConcat(mgr.author.ID, mgr.author.ID), r.Key)
	if err != nil {
		return nil, err
	}

	return initPublish, nil
}

const exampleAddMemberContent = `ex:
var content = {
	type: 'group/add-member',
	version: 'v1',
	groupKey: '3YUat1ylIUVGaCjotAvof09DhyFxE8iGbF6QxLlCWWc=',
	initialMsg: '%THxjTGPuXvvxnbnAV7xVuVXdhDcmoNtDDN0j3UTxcd8=.sha256',
	text: 'welcome keks!',                                      // optional
	recps: [
	  '%vof09Dhy3YUat1ylIUVGaCjotAFxE8iGbF6QxLlCWWc=.cloaked',  // group_id
	  '@YXkE3TikkY4GFMX3lzXUllRkNTbj5E+604AkaO1xbz8=.ed25519'   // feed_id (for new person)
	],
  
	tangles: {
	  group: {
		root: '%THxjTGPuXvvxnbnAV7xVuVXdhDcmoNtDDN0j3UTxcd8=.sha256',
		previous: [
		  '%Sp294oBk7OJxizvPOlm6Sqk3fFJA2EQFiyJ1MS/BZ9E=.sha256'
		]
	  },
	  members: {
		root: '%THxjTGPuXvvxnbnAV7xVuVXdhDcmoNtDDN0j3UTxcd8=.sha256',
		previous: [
		  '%lm6Sqk3fFJA2EQFiyJ1MSASDASDASDASDASDAS/BZ9E=.sha256',
		  '%Sp294oBk7OJxizvPOlm6Sqk3fFJA2EQFiyJ1MS/BZ9E=.sha256'
		]
	  }
	}
  }`

func (mgr *Manager) AddMember(r *refs.FeedRef) (*refs.MessageRef, error) {
	return nil, fmt.Errorf("TODO")
}
