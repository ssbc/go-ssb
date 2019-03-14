package imap

import (
	"log"
	"testing"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/server"
	"github.com/stretchr/testify/require"
)

func TestBackend(t *testing.T) {
	r := require.New(t)
	// Create SSB backend
	be := NewBackend()

	// Create a new server
	s := server.New(be)
	s.Addr = "localhost:1143"

	// Since we will use this server for testing only, we can allow plain text
	// authentication over unencrypted connections
	s.AllowInsecureAuth = true

	doneSrv := make(chan error, 1)
	go func() {
		doneSrv <- s.ListenAndServe()
	}()

	log.Println("Connecting to server...")

	// Connect to server
	c, err := client.Dial("localhost:1143")
	r.NoError(err)
	log.Println("Connected")

	// Don't forget to logout
	defer c.Logout()

	// Login
	err = c.Login("cryptix", "superSecure")
	r.NoError(err)
	log.Println("Logged in")

	// List mailboxes
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	log.Println("Mailboxes:")
	for m := range mailboxes {
		log.Println("* " + m.Name)
	}

	r.NoError(<-done)

	// Select INBOX
	mbox, err := c.Select("INBOX", false)
	r.NoError(err)
	log.Println("Flags for INBOX:", mbox.Flags)

	// Get the last 4 messages
	from := uint32(1)
	to := mbox.Messages
	if mbox.Messages > 3 {
		// We're using unsigned integers here, only substract if the result is > 0
		from = mbox.Messages - 3
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, 10)
	done = make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()

	log.Println("Last 4 messages:")
	for msg := range messages {
		log.Println("* " + msg.Envelope.Subject)
	}

	r.NoError(<-done)

	log.Println("Done!")
	r.NoError(<-doneSrv)
}
