// SPDX-FileCopyrightText: 2023 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ssbc/go-ssb/internal/config-reader"
	"github.com/stretchr/testify/require"
)

func TestMarshallConfig(t *testing.T) {
	r := require.New(t)
	configContents := `[sbotcli]
shscap = "C80ZE1AsIGuRehUpiHXCRt3akFJzUTKqXEJ7i30OjNI="
addr = "localhost:12345"
remotekey = "@QlCTpvY7p9ty2yOFrv1WU1AE88aoQc4Y7wYal7PFc+w=.ed25519"
key = "/tmp/testkey"
unixsock = "/tmp/testsocket"
timeout = "3600s"
`
	expectedConfig := config.SbotCliConfig{
		ShsCap:             "C80ZE1AsIGuRehUpiHXCRt3akFJzUTKqXEJ7i30OjNI=",
		Addr:               "localhost:12345",
		RemoteKey:          "@QlCTpvY7p9ty2yOFrv1WU1AE88aoQc4Y7wYal7PFc+w=.ed25519",
		Key:                "/tmp/testkey",
		UnixSock:           "/tmp/testsocket",
		Timeout:            "3600s",
	}
	testPath := filepath.Join(".", "testrun", t.Name())
	r.NoError(os.RemoveAll(testPath), "remove testrun folder")
	r.NoError(os.MkdirAll(testPath, 0700), "make new testrun folder")
	configPath := filepath.Join(testPath, "config.toml")
	err := os.WriteFile(configPath, []byte(configContents), 0700)
	r.NoError(err, "write config file")
	configFromDisk, _ := config.ReadConfigSbotCli(configPath)
	// config values should be read correctly
	r.EqualValues(expectedConfig.ShsCap, configFromDisk.ShsCap)
	r.EqualValues(expectedConfig.Addr, configFromDisk.Addr)
	r.EqualValues(expectedConfig.RemoteKey, configFromDisk.RemoteKey)
	r.EqualValues(expectedConfig.Key, configFromDisk.Key)
	r.EqualValues(expectedConfig.UnixSock, configFromDisk.UnixSock)
	r.EqualValues(expectedConfig.Timeout, configFromDisk.Timeout)
}

func TestUnmarshalConfig(t *testing.T) {
	r := require.New(t)
	config := config.SbotCliConfig{
		ShsCap:             "C80ZE1AsIGuRehUpiHXCRt3akFJzUTKqXEJ7i30OjNI=",
		Addr:               "localhost:12345",
		RemoteKey:          "@QlCTpvY7p9ty2yOFrv1WU1AE88aoQc4Y7wYal7PFc+w=.ed25519",
		Key:                "/tmp/testkey",
		UnixSock:           "/tmp/testsocket",
		Timeout:            "3600s",
	}
	b, err := json.MarshalIndent(config, "", "  ")
	r.NoError(err)
	configStr := string(b)
	expectedValues := strings.Split(strings.TrimSpace(`"shscap": "C80ZE1AsIGuRehUpiHXCRt3akFJzUTKqXEJ7i30OjNI=",
  "addr": "localhost:12345",
  "remotekey": "@QlCTpvY7p9ty2yOFrv1WU1AE88aoQc4Y7wYal7PFc+w=.ed25519",
  "key": "/tmp/testkey",
  "unixsock": "/tmp/testsocket",
  "timeout": "3600s"
	`), "\n")
	for _, expected := range expectedValues {
		r.True(strings.Contains(configStr, expected), expected)
	}
}

