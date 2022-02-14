package tests

import (
	"testing"
	"fmt"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/internal/storedrefs"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/sbot"
	"github.com/stretchr/testify/assert"
	refs "go.mindeco.de/ssb-refs"
)

func TestStartup (t *testing.T) {
	a := assert.New(t)
	var err error
	session := newSession(t, nil, nil)
	session.startGoBot()
	bot := session.gobot
	feedID := bot.KeyPair.ID()
	a.NotNil(bot)
	botlog, err := getFeed(bot, feedID)
	a.NoError(err)
	a.NotNil(botlog)
	a.EqualValues(-1, botlog.Seq(), "maggie seqno of fresh log should be -1")

	// note (2022-02-14): this is maybe not the same mechanism of publishing as the route used when running via muxrpcs?
	// post a message
	_, err = bot.PublishLog.Append(refs.Post{Type: "post", Text: "1 hello world!"})
	a.NoError(err)
	a.EqualValues(0, botlog.Seq(), "maggie seqno of log with 1 message should be 0")
	_, err = bot.PublishLog.Append(refs.Post{Type: "post", Text: "2 hello world!"})
	a.NoError(err)
	a.EqualValues(1, botlog.Seq(), "maggie seqno of log with 2 messages should be 1")
	// close the go bot
	bot.Shutdown()
	session.wait()
	// start the go bot again
	session.startGoBot()
	bot = session.gobot
	botlog, err = getFeed(bot, feedID)
	a.NoError(err)
	a.NotNil(botlog)
	a.EqualValues(1, botlog.Seq(), "maggie seqno of log with 2 messages should be 1")
	// post another message
	_, err = bot.PublishLog.Append(refs.Post{Type: "post", Text: "3 hello world!"})
	a.NoError(err)
	a.EqualValues(2, botlog.Seq(), "maggie seqno of log with 3 messages should be 2")
	// post another message
	_, err = bot.PublishLog.Append(refs.Post{Type: "post", Text: "4 hello world!"})
	a.NoError(err)
	a.EqualValues(3, botlog.Seq(), "maggie seqno of log with 4 messages should be 3")
	bot.Shutdown()
}

// error wrap - a helper util
// string header will be prefixed before each message. typically it is the context we're generating errors within.
// msg is the specific message, err is the error (if passed)
func ew(header string) func(msg string, err ...error) error {
    return func(msg string, err ...error) error {
        if len(err) > 0 {
            return fmt.Errorf("[gossb: %s] %s (%w)", header, msg, err[0])
        }
        return fmt.Errorf("[gossb: %s] %s", header, msg)
    }
}

func getFeed(bot *sbot.Sbot, feedID refs.FeedRef) (margaret.Log, error) {
    feed, err := bot.Users.Get(storedrefs.Feed(feedID))
    if err != nil {
        return nil, fmt.Errorf("get feed failed (%w)", err)
    }

    // convert from log of seqnos-in-rxlog to log of refs.Message and return
    return mutil.Indirect(bot.ReceiveLog, feed), nil
}
