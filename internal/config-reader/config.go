// SPDX-FileCopyrightText: 2023 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	loglib "log"
	"os"
	"path/filepath"
	"strings"

	"github.com/komkom/toml"
	"github.com/ssbc/go-ssb/internal/testutils"
	"go.mindeco.de/log/level"
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
	WebsocketTLSCert string `json:"wstlscert,omitempty"`
	WebsocketTLSKey  string `json:"wstlskey,omitempty"`
	MetricsAddress   string `json:"debuglis,omitempty"`

	NoUnixSocket        ConfigBool `json:"nounixsock"`
	EnableAdvertiseUDP  ConfigBool `json:"localadv"`
	EnableDiscoveryUDP  ConfigBool `json:"localdiscov"`
	EnableEBT           ConfigBool `json:"enable-ebt"`
	EnableFirewall      ConfigBool `json:"promisc"`
	RepairFSBeforeStart ConfigBool `json:"repair"`

	NumPeer uint `json:"numPeer,omitempty"`
	NumRepl uint `json:"numRepl,omitempty"`

	Presence map[string]interface{}
}
type SbotCliConfig struct {
	ShsCap    string `json:"shscap,omitempty"`
	Addr      string `json:"addr,omitempty"`
	RemoteKey string `json:"remotekey,omitempty"`
	Key       string `json:"key,omitempty"`
	UnixSock  string `json:"unixsock,omitempty"`
	Timeout   string `json:"timeout,omitempty"`

	Presence map[string]interface{}
}

type MergedConfig struct {
	GoSbot SbotConfig `json:"go-sbot"`
	SbotCli SbotCliConfig `json:"sbotcli"`
}

func (config SbotConfig) Has(flagname string) bool {
	_, ok := config.Presence[flagname]
	return ok
}

func ReadConfigSbot(configPath string) (SbotConfig, bool) {
	var conf MergedConfig

	// setup logger if not yet setup (used for tests)
	log := testutils.NewRelativeTimeLogger(nil)

	data, err := os.ReadFile(configPath)
	if err != nil {
		level.Info(log).Log("event", "read config", "msg", "no config detected", "path", configPath)
		return conf.GoSbot, false
	}

	level.Info(log).Log("event", "read config", "msg", "config detected", "path", configPath)

	// 1) first we unmarshal into struct for type checks
	decoder := json.NewDecoder(toml.New(bytes.NewBuffer(data)))
	err = decoder.Decode(&conf)
	check(err, "decode into struct")

	// 2) then we unmarshal into a map for presence check (to make sure bools are treated correctly)
	presence := make(map[string]interface{})
	decoder = json.NewDecoder(toml.New(bytes.NewBuffer(data)))
	err = decoder.Decode(&presence)
	check(err, "decode into presence map")
	if presence["go-sbot"] != nil {
		conf.GoSbot.Presence = presence["go-sbot"].(map[string]interface{})
	} else {
		level.Warn(log).Log("event", "read config", "msg", "no [go-sbot] detected in config file - I am not reading anything from the config file", "path", configPath)
		conf.GoSbot.Presence = make(map[string]interface{})
	}

	// help repo path's default to align with common user expectations
	conf.GoSbot.Repo = expandPath(conf.GoSbot.Repo)

	return conf.GoSbot, true
}

func ReadConfigSbotCli(configPath string) (SbotCliConfig, bool) {
	var conf MergedConfig

	// setup logger if not yet setup (used for tests)
	log := testutils.NewRelativeTimeLogger(nil)

	data, err := os.ReadFile(configPath)
	if err != nil {
		level.Info(log).Log("event", "read config", "msg", "no config detected", "path", configPath)
		return conf.SbotCli, false
	}

	level.Info(log).Log("event", "read config", "msg", "config detected", "path", configPath)

	// 1) first we unmarshal into struct for type checks
	decoder := json.NewDecoder(toml.New(bytes.NewBuffer(data)))
	err = decoder.Decode(&conf)
	check(err, "decode into struct")

	// 2) then we unmarshal into a map for presence check (to make sure bools are treated correctly)
	presence := make(map[string]interface{})
	decoder = json.NewDecoder(toml.New(bytes.NewBuffer(data)))
	err = decoder.Decode(&presence)
	check(err, "decode into presence map")
	if presence["sbotcli"] != nil {
		conf.SbotCli.Presence = presence["sbotcli"].(map[string]interface{})
	} else {
		level.Warn(log).Log("event", "read config", "msg", "no [sbotcli] detected in config file - I am not reading anything from the config file", "path", configPath)
		conf.SbotCli.Presence = make(map[string]interface{})
	}

	return conf.SbotCli, true
}

// ensure the following type of path expansions take place:
// * ~/.ssb				=> /home/<user>/.ssb
// * .ssb					=> /home/<user>/.ssb
// * /stuff/.ssb	=> /stuff/.ssb
func expandPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		loglib.Fatalln("could not get user home directory (os.UserHomeDir()")
	}

	if strings.HasPrefix(p, "~") {
		p = strings.Replace(p, "~", home, 1)
	}

	// not relative path, not absolute path =>
	// place relative to home dir "~/<here>"
	if !filepath.IsAbs(p) {
		p = filepath.Join(home, p)
	}

	return p
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
