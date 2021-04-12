package multilogs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

type Members map[string]bool

// MembershipStore isn't strictly a multilog but putting it in package private gave cyclic import
type MembershipStore struct {
	idx         librarian.SeqSetterIndex
	self        refs.FeedRef
	unboxer     *private.Manager
	combinedidx *CombinedIndex
}

var _ io.Closer = (*MembershipStore)(nil)

// NewMembershipIndex tracks group/add-member messages and triggers re-reading box2 messages by the invited people that couldn't be read before.
func NewMembershipIndex(r repo.Interface, self refs.FeedRef, unboxer *private.Manager, comb *CombinedIndex) (*MembershipStore, librarian.SinkIndex, error) {
	pth := r.GetPath(repo.PrefixIndex, "groups", "members", "mkv")
	err := os.MkdirAll(pth, 0700)
	if err != nil {
		return nil, nil, fmt.Errorf("openIndex: error making index directory: %w", err)
	}

	db, err := repo.OpenMKV(pth)
	if err != nil {
		return nil, nil, fmt.Errorf("openIndex: failed to open MKV database: %w", err)
	}
	var store = MembershipStore{
		idx:         libmkv.NewIndex(db, Members{}),
		self:        self,
		unboxer:     unboxer,
		combinedidx: comb,
	}

	snk := librarian.NewSinkIndex(store.updateFn, store.idx)
	return &store, snk, nil
}

func (mc MembershipStore) Close() error {
	return mc.idx.Close()
}

func (mc MembershipStore) updateFn(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
	msg, ok := val.(refs.Message)
	if !ok {
		return fmt.Errorf("not a message: %T", val)
	}

	if msg.Author().Equal(mc.self) {
		// our own message - all is done already
		fmt.Println("skipping invite from self")
		return nil
	}

	cleartext, err := mc.unboxer.DecryptMessage(msg)
	if err != nil {
		return nil // invalid message
	}

	var addMemberMsg private.GroupAddMember
	err = json.Unmarshal(cleartext, &addMemberMsg)
	if err != nil {
		return nil // invalid message
	}

	var groupID refs.MessageRef
	var newMembers []refs.FeedRef
	for _, r := range addMemberMsg.Recps {
		rcp, err := refs.ParseMessageRef(r)
		if err == nil && rcp.Algo() == refs.RefAlgoCloakedGroup {
			groupID = rcp
			continue
		}

		m, err := refs.ParseFeedRef(r)
		if err != nil {
			return nil // invalid message
		}
		newMembers = append(newMembers, m)
	}

	/* TODO? not really required but would fit into the existing scheme
	   then again, we would need to allocate a value in tfk for this...

		groupAsTFK, err := tfk.Encode(groupID)
		if err != nil {
			return err
		}
	*/

	idxAddr := storedrefs.Message(groupID)
	state, err := mc.idx.Get(ctx, idxAddr)
	if err != nil {
		return err
	}

	statev, err := state.Value()
	if err != nil {
		return err
	}

	var currentMembers Members
	switch tv := statev.(type) {
	case Members:
		currentMembers = tv
	case librarian.UnsetValue:
		currentMembers = make(Members, 0)
	default:
		return fmt.Errorf("not a Member: %T", statev)
	}

	for _, nm := range newMembers {
		_, indexed := currentMembers[nm.Ref()]
		if indexed {
			// already processed
			continue
		}

		whoToIndex := nm
		if nm.Equal(mc.self) {
			// if the invite is for us, we need to add the new group key
			cloakedGroupID, err := mc.unboxer.Join(addMemberMsg.GroupKey, addMemberMsg.Root)
			if err != nil {
				return err
			}
			fmt.Println("joined group:", cloakedGroupID.Ref())

			// if we are invited, we need to index the sending author
			whoToIndex = msg.Author()
		}

		err = mc.combinedidx.Box2Reindex(whoToIndex)
		if err != nil {
			return err
		}

		// mark as indexed
		currentMembers[whoToIndex.Ref()] = true
	}

	err = mc.idx.Set(ctx, idxAddr, currentMembers)
	if err != nil {
		return err
	}

	return nil
}
