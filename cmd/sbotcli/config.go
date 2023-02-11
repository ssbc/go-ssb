// SPDX-FileCopyrightText: 2023 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	loglib "log"
	"os"

	"github.com/ssbc/go-ssb/internal/config-reader"
)

func ReadEnvironmentVariables(config *config.SbotCliConfig) {
	if val := os.Getenv("SSB_CAP_SHS_KEY"); val != "" {
		config.ShsCap = val
		config.Presence["shscap"] = true
	}

	if val := os.Getenv("SSB_ADDR"); val != "" {
		config.Addr = val
		config.Presence["addr"] = true
	}

	if val := os.Getenv("SSB_REMOTE_KEY"); val != "" {
		config.RemoteKey = val
		config.Presence["remotekey"] = true
	}

	if val := os.Getenv("SSB_KEY"); val != "" {
		config.Key = val
		config.Presence["key"] = true
	}

	if val := os.Getenv("SSB_UNIX_SOCK"); val != "" {
		config.UnixSock = val
		config.Presence["unixsock"] = true
	}

	if val := os.Getenv("SSB_TIMEOUT"); val != "" {
		config.Timeout = val
		config.Presence["timeout"] = true
	}
}

func readEnvironmentBoolean(s string) config.ConfigBool {
	var booly config.ConfigBool
	err := json.Unmarshal([]byte(s), booly)
	configCheck(err, "parsing environment variable bool")
	return booly
}

func readConfigAndEnv(configPath string) (config.SbotCliConfig, bool) {
	config, exists := config.ReadConfigSbotCli(configPath)
	ReadEnvironmentVariables(&config)
	return config, exists
}

func eout(err error, msg string, args ...interface{}) error {
	if err != nil {
		msg = fmt.Sprintf(msg, args...)
		return fmt.Errorf("%s (%w)", msg, err)
	}
	return nil
}

func configCheck(err error, msg string, args ...interface{}) {
	if err = eout(err, msg, args...); err != nil {
		loglib.Fatalln(err)
	}
}
