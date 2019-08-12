package names_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/plugins2/names"
	"go.cryptoscope.co/ssb/sbot"
)

func TestAboutNames(t *testing.T) {
	r := require.New(t)
	ctx := context.TODO()

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	repoPath := filepath.Join("testrun", t.Name(), "about")
	os.RemoveAll(repoPath)

	aliLog, _ := logtest.KitLogger("ali", t)
	ali, err := sbot.New(
		sbot.WithHMACSigning(hk),
		sbot.WithInfo(aliLog),
		sbot.WithRepoPath(repoPath),
		sbot.WithUNIXSocket(),
		sbot.LateOption(sbot.MountPlugin(&names.Plugin{}, plugins2.AuthMaster)),
	)
	r.NoError(err)

	var aliErrc = make(chan error, 1)
	go func() {
		err := ali.Network.Serve(ctx)
		if err != nil {
			aliErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(aliErrc)
	}()
	var newName ssb.About
	newName.Type = "about"
	newName.Name = fmt.Sprintf("testName:%x", hk[:16])
	newName.About = ali.KeyPair.Id

	_, err = ali.PublishLog.Append(newName)
	r.NoError(err)

	src, err := ali.RootLog.Query()
	r.NoError(err)
	var i = 0
	for {
		v, err := src.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		}
		sm := v.(ssb.Message)
		var a ssb.About
		c := sm.ContentBytes()
		err = json.Unmarshal(c, &a)
		r.NoError(err)
		r.Equal(newName.Name, a.Name)
		i++
	}
	r.Equal(i, 1)

	c, err := client.NewUnix(context.TODO(), filepath.Join(repoPath, "socket"))
	r.NoError(err)

	all, err := c.NamesGet()
	r.NoError(err)

	name, ok := all.GetCommonName(ali.KeyPair.Id)
	r.True(ok)
	r.Equal(newName.Name, name)

	name2, err := c.NamesSignifier(*ali.KeyPair.Id)
	r.NoError(err)
	r.Equal(newName.Name, name2)

	r.NoError(c.Close())

	ali.Shutdown()
	r.NoError(ali.Close())
	r.NoError(<-aliErrc)
}
