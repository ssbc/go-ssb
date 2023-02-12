// SPDX-FileCopyrightText: 2023 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	loglib "log"
	"os"
	"strconv"

	"github.com/ssbc/go-ssb/internal/config-reader"
)

func ReadEnvironmentVariables(config *config.SbotConfig) {
	if val := os.Getenv("SSB_SECRET_FILE"); val != "" {
		loglib.Fatalln("flag SSB_SECRET_FILE not implemented")
	}

	if val := os.Getenv("SSB_SOCKET_FILE"); val != "" {
		loglib.Fatalln("flag SSB_SOCKET_FILE not implemented")
	}

	if val := os.Getenv("SSB_LOG_LEVEL"); val != "" {
		loglib.Fatalln("flag SSB_LOG_LEVEL not implemented")
	}

	if val := os.Getenv("SSB_CAP_INVITE_KEY"); val != "" {
		loglib.Fatalln("flag SSB_CAP_INVITE_KEY not implemented")
	}

	// go-ssb specific env flag, for peachcloud/pub compat
	if val := os.Getenv("GO_SSB_REPAIR_FS"); val != "" {
		config.RepairFSBeforeStart = readEnvironmentBoolean(val)
		config.SetPresence("repair", true)
	}

	if val := os.Getenv("SSB_DATA_DIR"); val != "" {
		config.Repo = val
		config.SetPresence("repo", true)
	}

	if val := os.Getenv("SSB_LOG_DIR"); val != "" {
		config.DebugDir = val
		config.SetPresence("debugdir", true)
	}

	if val := os.Getenv("SSB_PROMETHEUS_ADDRESS"); val != "" {
		config.MetricsAddress = val
		config.SetPresence("debuglis", true)
	}

	if val := os.Getenv("SSB_PROMETHEUS_ENABLED"); val != "" {
		config.SetPresence("debuglis", readEnvironmentBoolean(val))
	}

	if val := os.Getenv("SSB_HOPS"); val != "" {
		hops, err := strconv.Atoi(val)
		check(err, "parse hops from environment variable")
		config.Hops = uint(hops)
		config.SetPresence("hops", true)
	}

	if val := os.Getenv("SSB_MUXRPC_ADDRESS"); val != "" {
		config.MuxRPCAddress = val
		config.SetPresence("lis", true)
	}

	if val := os.Getenv("SSB_WS_ADDRESS"); val != "" {
		config.WebsocketAddress = val
		config.SetPresence("wslis", true)
	}

	if val := os.Getenv("SSB_WS_TLS_CERT"); val != "" {
		config.WebsocketTLSCert = val
		config.SetPresence("wstlscert", true)
	}

	if val := os.Getenv("SSB_WS_TLS_KEY"); val != "" {
		config.WebsocketTLSKey = val
		config.SetPresence("wstlskey", true)
	}

	if val := os.Getenv("SSB_EBT_ENABLED"); val != "" {
		config.EnableEBT = readEnvironmentBoolean(val)
		config.SetPresence("enable-ebt", true)
	}

	if val := os.Getenv("SSB_CONN_FIREWALL_ENABLED"); val != "" {
		config.EnableFirewall = readEnvironmentBoolean(val)
		config.SetPresence("promisc", true)
	}

	if val := os.Getenv("SSB_SOCKET_ENABLED"); val != "" {
		config.NoUnixSocket = !readEnvironmentBoolean(val)
		config.SetPresence("nounixsock", true)
	}

	if val := os.Getenv("SSB_CONN_DISCOVERY_UDP_ENABLED"); val != "" {
		config.EnableDiscoveryUDP = readEnvironmentBoolean(val)
		config.SetPresence("localdiscov", true)
	}

	if val := os.Getenv("SSB_CONN_BROADCAST_UDP_ENABLED"); val != "" {
		config.EnableAdvertiseUDP = readEnvironmentBoolean(val)
		config.SetPresence("localadv", true)
	}

	if val := os.Getenv("SSB_CAP_SHS_KEY"); val != "" {
		config.ShsCap = val
		config.SetPresence("shscap", true)
	}

	if val := os.Getenv("SSB_CAP_HMAC_KEY"); val != "" {
		config.Hmac = val
		config.SetPresence("hmac", true)
	}

	if val := os.Getenv("SSB_NUM_PEER"); val != "" {
		numPeer, err := strconv.Atoi(val)
		check(err, "parse numPeer from environment variable")
		config.NumPeer = uint(numPeer)
	}

	if val := os.Getenv("SSB_NUM_REPL"); val != "" {
		numRepl, err := strconv.Atoi(val)
		check(err, "parse numRepl from environment variable")
		config.NumRepl = uint(numRepl)
	}
}

func readEnvironmentBoolean(s string) config.ConfigBool {
	var booly config.ConfigBool
	err := json.Unmarshal([]byte(s), booly)
	check(err, "parsing environment variable bool")
	return booly
}

func readConfigAndEnv(configPath string) (config.SbotConfig, bool) {
	config, exists := config.ReadConfigSbot(configPath)
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

func check(err error, msg string, args ...interface{}) {
	if err = eout(err, msg, args...); err != nil {
		loglib.Fatalln(err)
	}
}
