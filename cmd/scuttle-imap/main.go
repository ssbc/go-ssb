package main

import (
	"errors"
	"flag"
	"log"

	"github.com/emersion/go-imap/server"
	"go.cryptoscope.co/ssb/cmd/scuttle-imap/imap"
)

var ErrTODO = errors.New("TODO")

var listenAddr string

func init() {
	flag.StringVar(&listenAddr, "l", ":1143", "address to listen on")
}

func main() {
	flag.Parse()

	// Create SSB backend
	be := imap.NewBackend()

	// Create a new server
	s := server.New(be)
	s.Addr = listenAddr

	// Since we will use this server for testing only, we can allow plain text
	// authentication over unencrypted connections
	s.AllowInsecureAuth = true

	log.Printf("Starting IMAP server at %s", listenAddr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
