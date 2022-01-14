package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/komkom/toml"
	"go.mindeco.de/log/level"
	loglib "log"
	"os"
	"strconv"
)

type SbotConfig struct {
	ShsCap string `json:"shscap"`
	Hmac   string `json:"hmac"`
	Hops   uint   `json:"hops"`

	Repo     string `json:"repo"`
	DebugDir string `json:"debugdir"`

	MuxRPCAddress    string `json:"lis"`
	WebsocketAddress string `json:"wslis"`
	MetricsAddress   string `json:"debuglis"`

	NoUnixSocket       bool `json:"nounixsock"`
	EnableAdvertiseUDP bool `json:"localdv"`
	EnableDiscoveryUDP bool `json:"localdiscov"`
	EnableEBT          bool `json:"enable-ebt"`
	EnableFirewall     bool `json:"promisc"`

	presence map[string]interface{}
}

func (config SbotConfig) Has(flagname string) bool {
	_, ok := config.presence[flagname]
	return ok
}

func ReadConfig(configPath string) SbotConfig {
	data, err := os.ReadFile(configPath)
	var msg string
	if err != nil {
		msg = fmt.Sprintf("no config detected at %s; running without it", configPath)
		level.Info(log).Log("event", msg)
		return SbotConfig{}
	} else {
	}
	msg = fmt.Sprintf("config detected (%s)", configPath)
	level.Info(log).Log("event", msg)

	// 1) first we unmarshal into struct for type checks
	var conf SbotConfig
	decoder := json.NewDecoder(toml.New(bytes.NewBuffer(data)))
	err = decoder.Decode(&conf)
	check(err, "decode into struct")

	// 2) then we unmarshal into a map for presence check (to make sure bools are treated correctly)
	decoder = json.NewDecoder(toml.New(bytes.NewBuffer(data)))
	err = decoder.Decode(&conf.presence)
	check(err, "decode into presence map")

	return conf
}

func ReadEnvironmentVariables(config *SbotConfig) {
	if val := os.Getenv("SSB_DATA_DIR"); val != "" {
		config.Repo = val
		config.presence["repo"] = true
	}

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

	// handled in /cmd/go-sbot
	// if val := os.Getenv("SSB_CONFIG_FILE"); val != "" {
	// }

	if val := os.Getenv("SSB_LOG_DIR"); val != "" {
		config.DebugDir = val
		config.presence["debugdir"] = true
	}
	if val := os.Getenv("SSB_PROMETHEUS_ADDRESS"); val != "" {
		config.MetricsAddress = val
		config.presence["debuglis"] = true
	}
	if val := os.Getenv("SSB_PROMETHEUS_ENABLED"); val != "" {
		config.presence["debuglis"] = BooleanIsTrue(val)
	}

	if val := os.Getenv("SSB_HOPS"); val != "" {
		hops, err := strconv.Atoi(val)
		check(err, "parse hops from environment variable")
		config.Hops = uint(hops)
		config.presence["hops"] = true
	}
	if val := os.Getenv("SSB_MUXRPC_ADDRESS"); val != "" {
		config.MuxRPCAddress = val
		config.presence["lis"] = true
	}
	if val := os.Getenv("SSB_WS_ADDRESS"); val != "" {
		config.WebsocketAddress = val
		config.presence["wslis"] = true
	}
	if val := os.Getenv("SSB_EBT_ENABLED"); val != "" {
		config.EnableEBT = BooleanIsTrue(val)
		config.presence["enable-ebt"] = true
	}
	if val := os.Getenv("SSB_CONN_FIREWALL_ENABLED"); val != "" {
		config.EnableFirewall = BooleanIsTrue(val)
		config.presence["promisc"] = true
	}
	if val := os.Getenv("SSB_SOCKET_ENABLED"); val != "" {
		config.NoUnixSocket = !BooleanIsTrue(val)
		config.presence["nounixsock"] = true
	}
	if val := os.Getenv("SSB_CONN_DISCOVERY_UDP_ENABLED"); val != "" {
		config.EnableDiscoveryUDP = BooleanIsTrue(val)
		config.presence["localdiscov"] = true
	}
	if val := os.Getenv("SSB_CONN_BROADCAST_UDP_ENABLED"); val != "" {
		config.EnableAdvertiseUDP = BooleanIsTrue(val)
		config.presence["localadv"] = true
	}
	if val := os.Getenv("SSB_CAP_SHS_KEY"); val != "" {
		config.ShsCap = val
		config.presence["shscap"] = true
	}
	if val := os.Getenv("SSB_CAP_HMAC_KEY"); val != "" {
		config.Hmac = val
		config.presence["hmac"] = true
	}
}

func BooleanIsTrue(s string) bool {
	return s == "1" || s == "yes" || s == "on"
}

func ReadConfigAndEnv(configPath string) SbotConfig {
	config := ReadConfig(configPath)
	ReadEnvironmentVariables(&config)
	return config
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
