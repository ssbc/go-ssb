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

	Tangles refs.Tangles `json:"tangles"`
}

// Create returns cloaked id and public root of a new group
func (mgr *Manager) Create(name string) (*refs.MessageRef, *refs.MessageRef, error) {
	// roll new key
	var r keys.Recipient
	r.Scheme = keys.SchemeLargeSymmetricGroup
	r.Key = make([]byte, 32) // TODO: key size const
	_, err := rand.Read(r.Key)
	if err != nil {
		return nil, nil, err
	}

	// prepare init content
	var gi groupInit
	gi.Type = "group/init"
	gi.Name = name
	gi.Tangles = make(refs.Tangles)
	gi.Tangles["group"] = refs.TanglePoint{}   // empty/root
	gi.Tangles["members"] = refs.TanglePoint{} // empty/root

	jsonContent, err := json.Marshal(gi)
	if err != nil {
		return nil, nil, err
	}

	// encrypt the group/init message
	publicRoot, err := mgr.encryptAndPublish(jsonContent, keys.Recipients{r})
	if err != nil {
		return nil, nil, err
	}

	r.Metadata.GroupRoot = publicRoot

	cloakedID, err := mgr.deriveCloakedAndStoreNewKey(r)
	if err != nil {
		return nil, nil, err
	}

	return cloakedID, publicRoot, nil
}

func (mgr *Manager) Join(groupKey []byte, root *refs.MessageRef) (*refs.MessageRef, error) {
	var r keys.Recipient
	r.Scheme = keys.SchemeLargeSymmetricGroup
	r.Key = make([]byte, 32) // TODO: key size const

	if n := len(groupKey); n != 32 {
		return nil, fmt.Errorf("groups/join: passed key length (%d)", n)
	}
	copy(r.Key, groupKey)

	r.Metadata.GroupRoot = root

	cloakedID, err := mgr.deriveCloakedAndStoreNewKey(r)
	if err != nil {
		return nil, err
	}

	return cloakedID, nil
}

func (mgr *Manager) deriveCloakedAndStoreNewKey(k keys.Recipient) (*refs.MessageRef, error) {
	var cloakedID refs.MessageRef
	cloakedID.Algo = refs.RefAlgoCloakedGroup
	cloakedID.Hash = make([]byte, 32)

	err := box2.DeriveTo(cloakedID.Hash, k.Key, []byte("cloaked_msg_id"), k.Metadata.GroupRoot.Hash)
	if err != nil {
		return nil, err
	}

	err = mgr.keymgr.AddKey(cloakedID.Hash, k)
	if err != nil {
		return nil, err
	}

	err = mgr.keymgr.AddKey(sortAndConcat(mgr.author.Id.ID, mgr.author.Id.ID), k)
	if err != nil {
		return nil, err
	}

	return &cloakedID, nil
}

/*
{
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
}
*/

type GroupAddMember struct {
	Type string `json:"type"`
	Text string `json:"text"`

	Version string `json:"version"`

	GroupKey       keys.Base64String `json:"groupKey"`
	InitialMessage *refs.MessageRef  `json:"initialMsg"`

	Recps []string `json:"recps"`

	Tangles refs.Tangles `json:"tangles"`
}

func (mgr *Manager) AddMember(groupID *refs.MessageRef, r *refs.FeedRef, welcome string) (*refs.MessageRef, error) {
	if groupID.Algo != refs.RefAlgoCloakedGroup {
		return nil, fmt.Errorf("not a group")
	}

	gskey, err := mgr.keymgr.GetKeys(keys.SchemeLargeSymmetricGroup, groupID.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get key for group: %w", err)
	}

	if n := len(gskey); n != 1 {
		return nil, fmt.Errorf("inconsistent group-key count: %d", n)
	}

	sk, err := mgr.GetOrDeriveKeyFor(r)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key for feed: %w", err)
	}
	gskey = append(gskey, sk...)

	// prepare init content
	var ga GroupAddMember
	ga.Type = "group/add-member"
	ga.Version = "v1"
	ga.Text = welcome

	ga.GroupKey = keys.Base64String(gskey[0].Key)
	groupRoot := gskey[0].Metadata.GroupRoot
	ga.InitialMessage = groupRoot

	ga.Recps = []string{groupID.Ref(), r.Ref()}

	ga.Tangles = make(refs.Tangles)

	ga.Tangles["group"] = mgr.getTangleState(groupRoot, "group")
	ga.Tangles["members"] = mgr.getTangleState(groupRoot, "members")

	jsonContent, err := json.Marshal(ga)
	if err != nil {
		return nil, err
	}

	return mgr.encryptAndPublish(jsonContent, gskey)
}

func (mgr *Manager) PublishTo(groupID *refs.MessageRef, content []byte) (*refs.MessageRef, error) {
	if groupID.Algo != refs.RefAlgoCloakedGroup {
		return nil, fmt.Errorf("not a group")
	}
	r, err := mgr.keymgr.GetKeys(keys.SchemeLargeSymmetricGroup, groupID.Hash)
	if err != nil {
		return nil, err
	}

	return mgr.encryptAndPublish(content, r)
}

func (mgr *Manager) PublishPostTo(groupID *refs.MessageRef, text string) (*refs.MessageRef, error) {
	if groupID.Algo != refs.RefAlgoCloakedGroup {
		return nil, fmt.Errorf("not a group")
	}
	rs, err := mgr.keymgr.GetKeys(keys.SchemeLargeSymmetricGroup, groupID.Hash)
	if err != nil {
		return nil, err
	}

	if nr := len(rs); nr != 1 {
		return nil, fmt.Errorf("expected 1 key for group, got %d", nr)
	}
	r := rs[0]

	var p refs.Post
	p.Type = "post"
	p.Text = text
	p.Recps = refs.MessageRefs{groupID}
	p.Tangles = make(refs.Tangles)

	p.Tangles["group"] = mgr.getTangleState(r.Metadata.GroupRoot, "group")

	content, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return mgr.encryptAndPublish(content, rs)
}

// utils

// TODO: protect against race of changing previous
func (mgr *Manager) encryptAndPublish(c []byte, recps keys.Recipients) (*refs.MessageRef, error) {
	prev, err := mgr.getPrevious()
	if err != nil {
		return nil, err
	}
	// now create the ciphertext
	bxr := box2.NewBoxer(mgr.rand)
	ciphertext, err := bxr.Encrypt(c, mgr.author.Id, prev, recps)
	if err != nil {
		return nil, err
	}

	return mgr.publishCiphertext(ciphertext)
}

func (mgr *Manager) getPrevious() (*refs.MessageRef, error) {
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
	return prev, nil
}

func (mgr *Manager) publishCiphertext(ctxt []byte) (*refs.MessageRef, error) {
	// TODO: format check for gabbygrove
	content := base64.StdEncoding.EncodeToString(ctxt) + ".box2"

	r, err := mgr.publog.Publish(content)
	if err != nil {
		return nil, err
	}
	return r, nil
}
