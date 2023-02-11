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
		config.Presence["repair"] = true
	}

	if val := os.Getenv("SSB_DATA_DIR"); val != "" {
		config.Repo = val
		config.Presence["repo"] = true
	}

	if val := os.Getenv("SSB_LOG_DIR"); val != "" {
		config.DebugDir = val
		config.Presence["debugdir"] = true
	}

	if val := os.Getenv("SSB_PROMETHEUS_ADDRESS"); val != "" {
		config.MetricsAddress = val
		config.Presence["debuglis"] = true
	}

	if val := os.Getenv("SSB_PROMETHEUS_ENABLED"); val != "" {
		config.Presence["debuglis"] = readEnvironmentBoolean(val)
	}

	if val := os.Getenv("SSB_HOPS"); val != "" {
		hops, err := strconv.Atoi(val)
		check(err, "parse hops from environment variable")
		config.Hops = uint(hops)
		config.Presence["hops"] = true
	}

	if val := os.Getenv("SSB_MUXRPC_ADDRESS"); val != "" {
		config.MuxRPCAddress = val
		config.Presence["lis"] = true
	}

	if val := os.Getenv("SSB_WS_ADDRESS"); val != "" {
		config.WebsocketAddress = val
		config.Presence["wslis"] = true
	}

	if val := os.Getenv("SSB_WS_TLS_CERT"); val != "" {
		config.WebsocketTLSCert = val
		config.Presence["wstlscert"] = true
	}

	if val := os.Getenv("SSB_WS_TLS_KEY"); val != "" {
		config.WebsocketTLSKey = val
		config.Presence["wstlskey"] = true
	}

	if val := os.Getenv("SSB_EBT_ENABLED"); val != "" {
		config.EnableEBT = readEnvironmentBoolean(val)
		config.Presence["enable-ebt"] = true
	}

	if val := os.Getenv("SSB_CONN_FIREWALL_ENABLED"); val != "" {
		config.EnableFirewall = readEnvironmentBoolean(val)
		config.Presence["promisc"] = true
	}

	if val := os.Getenv("SSB_SOCKET_ENABLED"); val != "" {
		config.NoUnixSocket = !readEnvironmentBoolean(val)
		config.Presence["nounixsock"] = true
	}

	if val := os.Getenv("SSB_CONN_DISCOVERY_UDP_ENABLED"); val != "" {
		config.EnableDiscoveryUDP = readEnvironmentBoolean(val)
		config.Presence["localdiscov"] = true
	}

	if val := os.Getenv("SSB_CONN_BROADCAST_UDP_ENABLED"); val != "" {
		config.EnableAdvertiseUDP = readEnvironmentBoolean(val)
		config.Presence["localadv"] = true
	}

	if val := os.Getenv("SSB_CAP_SHS_KEY"); val != "" {
		config.ShsCap = val
		config.Presence["shscap"] = true
	}

	if val := os.Getenv("SSB_CAP_HMAC_KEY"); val != "" {
		config.Hmac = val
		config.Presence["hmac"] = true
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
