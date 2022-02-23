package main

import (
	"bytes"
	"go.cryptoscope.co/ssb/internal/testutils"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/komkom/toml"
	"go.mindeco.de/log/level"
	loglib "log"
	"os"
	"strconv"
)

type ConfigBool bool
type SbotConfig struct {
	ShsCap string `json:"shscap,omitempty"`
	Hmac   string `json:"hmac,omitempty"`
	Hops   uint   `json:"hops,omitempty"`

	Repo     string `json:"repo,omitempty"`
	DebugDir string `json:"debugdir,omitempty"`

	MuxRPCAddress    string `json:"lis,omitempty"`
	WebsocketAddress string `json:"wslis,omitempty"`
	MetricsAddress   string `json:"debuglis,omitempty"`

	NoUnixSocket        ConfigBool `json:"nounixsock"`
	EnableAdvertiseUDP  ConfigBool `json:"localadv"`
	EnableDiscoveryUDP  ConfigBool `json:"localdiscov"`
	EnableEBT           ConfigBool `json:"enable-ebt"`
	EnableFirewall      ConfigBool `json:"promisc"`
	RepairFSBeforeStart ConfigBool `json:"repair"`

	presence map[string]interface{}
}

func (config SbotConfig) Has(flagname string) bool {
	_, ok := config.presence[flagname]
	return ok
}

func readConfig(configPath string) SbotConfig {
	var conf SbotConfig
	conf.presence = make(map[string]interface{})

	// setup logger if not yet setup (used for tests)
	if log == nil {
		log = testutils.NewRelativeTimeLogger(nil)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		level.Info(log).Log("event", "read config", "msg", "no config detected", "path", configPath)
		return conf
	}
	level.Info(log).Log("event", "read config", "msg", "config detected", "path", configPath)

	// 1) first we unmarshal into struct for type checks
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
		config.presence["repair"] = true
	}

	if val := os.Getenv("SSB_DATA_DIR"); val != "" {
		config.Repo = val
		config.presence["repo"] = true
	}

	if val := os.Getenv("SSB_LOG_DIR"); val != "" {
		config.DebugDir = val
		config.presence["debugdir"] = true
	}

	if val := os.Getenv("SSB_PROMETHEUS_ADDRESS"); val != "" {
		config.MetricsAddress = val
		config.presence["debuglis"] = true
	}

	if val := os.Getenv("SSB_PROMETHEUS_ENABLED"); val != "" {
		config.presence["debuglis"] = readEnvironmentBoolean(val)
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
		config.EnableEBT = readEnvironmentBoolean(val)
		config.presence["enable-ebt"] = true
	}

	if val := os.Getenv("SSB_CONN_FIREWALL_ENABLED"); val != "" {
		config.EnableFirewall = readEnvironmentBoolean(val)
		config.presence["promisc"] = true
	}

	if val := os.Getenv("SSB_SOCKET_ENABLED"); val != "" {
		config.NoUnixSocket = !readEnvironmentBoolean(val)
		config.presence["nounixsock"] = true
	}

	if val := os.Getenv("SSB_CONN_DISCOVERY_UDP_ENABLED"); val != "" {
		config.EnableDiscoveryUDP = readEnvironmentBoolean(val)
		config.presence["localdiscov"] = true
	}

	if val := os.Getenv("SSB_CONN_BROADCAST_UDP_ENABLED"); val != "" {
		config.EnableAdvertiseUDP = readEnvironmentBoolean(val)
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

func (booly ConfigBool) MarshalJSON() ([]byte, error) {
	temp := (bool)(booly)
	b, err := json.Marshal(temp)
	return b, err
}

func (booly *ConfigBool) UnmarshalJSON(b []byte) error {
	// unmarshal into interface{} first, as a bool can't be unmarshaled into a string
	var v interface{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return eout(err, "unmarshal config bool")
	}

	// go through a type assertion dance, capturing the two cases:
	// 1. if the config value is a proper boolean, and
	// 2. if the config value is a boolish string (e.g. "true" or "1")
	var temp bool
	if val, ok := v.(bool); ok {
		temp = val
	} else if s, ok := v.(string); ok {
		temp = booleanIsTrue(s)
		if !temp {
			// catch strings that cause a false value, but which aren't boolish
			if s != "false" && s != "0" && s != "no" && s != "off" {
				return errors.New("non-boolean string found when unmarshaling boolish values")
			}
		}
	}
	*booly = (ConfigBool)(temp)

	return nil
}

func booleanIsTrue(s string) bool {
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

func readEnvironmentBoolean(s string) ConfigBool {
	var booly ConfigBool
	err := json.Unmarshal([]byte(s), booly)
	check(err, "parsing environment variable bool")
	return booly
}

func readConfigAndEnv(configPath string) SbotConfig {
	config := readConfig(configPath)
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
