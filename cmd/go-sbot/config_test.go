package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMarshalConfigBooleans(t *testing.T) {
	r := require.New(t)
	configContents := `# Supply various flags to control go-sbot options.
hops = 2 

# Address to listen on
lis = ":8008" 
# Address to listen on for ssb websocket connections
wslis = ":8989" 

# Enable sending local UDP broadcasts
localadv = "on"
# Enable connecting to incoming UDP broadcasts
localdiscov = true
# Enable syncing by using epidemic-broadcast-trees (EBT)
enable-ebt = false 
# Bypass graph auth and fetch remote's feed, useful for pubs that are restoring their data from peers. Caveats abound, however.
promisc = false 
# Disable the UNIX socket RPC interface
nounixsock = false 

# how many feeds can be replicated with one peer connection using legacy gossip replication (shouldn't be higher than numRepl)
numPeer = 5
# how many feeds can be replicated concurrently using legacy gossip replication
numRepl = 10
`
	expectedConfig := SbotConfig{
		Hops:               2,
		MuxRPCAddress:      ":8008",
		WebsocketAddress:   ":8989",
		EnableAdvertiseUDP: true,
		EnableDiscoveryUDP: true,
		EnableEBT:          false,
		EnableFirewall:     false,
		NoUnixSocket:       false,
		NumPeer:            5,
		NumRepl:            10,
	}
	testPath := filepath.Join(".", "testrun", t.Name())
	r.NoError(os.RemoveAll(testPath), "remove testrun folder")
	r.NoError(os.MkdirAll(testPath, 0700), "make new testrun folder")
	configPath := filepath.Join(testPath, "config.toml")
	err := os.WriteFile(configPath, []byte(configContents), 0700)
	r.NoError(err, "write config file")
	configFromDisk := readConfig(configPath)
	// config values should be read correctly
	r.EqualValues(expectedConfig.Hops, configFromDisk.Hops)
	r.EqualValues(expectedConfig.MuxRPCAddress, configFromDisk.MuxRPCAddress)
	r.EqualValues(expectedConfig.WebsocketAddress, configFromDisk.WebsocketAddress)
	r.EqualValues(expectedConfig.EnableAdvertiseUDP, configFromDisk.EnableAdvertiseUDP)
	r.EqualValues(expectedConfig.EnableDiscoveryUDP, configFromDisk.EnableDiscoveryUDP)
	r.EqualValues(expectedConfig.EnableEBT, configFromDisk.EnableEBT)
	r.EqualValues(expectedConfig.EnableFirewall, configFromDisk.EnableFirewall)
	r.EqualValues(expectedConfig.NoUnixSocket, configFromDisk.NoUnixSocket)
	r.EqualValues(expectedConfig.NumPeer, configFromDisk.NumPeer)
	r.EqualValues(expectedConfig.NumRepl, configFromDisk.NumRepl)
}

func TestUnmarshalConfig(t *testing.T) {
	r := require.New(t)
	config := SbotConfig{
		NoUnixSocket:        true,
		EnableAdvertiseUDP:  true,
		EnableDiscoveryUDP:  true,
		EnableEBT:           true,
		EnableFirewall:      true,
		RepairFSBeforeStart: true,
		NumPeer:             5,
		NumRepl:             10,
	}
	b, err := json.MarshalIndent(config, "", "  ")
	r.NoError(err)
	configStr := string(b)
	expectedValues := strings.Split(strings.TrimSpace(`"nounixsock": true,
  "localadv": true,
  "localdiscov": true,
  "enable-ebt": true,
  "promisc": true,
  "repair": true,
  "numPeer": 5,
  "numRepl": 10
	`), "\n")
	for _, expected := range expectedValues {
		r.True(strings.Contains(configStr, expected), expected)
	}
}

func TestConfiguredSbot(t *testing.T) {
	r := require.New(t)
	configContents := `# Supply various flags to control go-sbot options.
hops = 2 

shscap = "0KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=" 
# Address to listen on
lis = ":8008" 
# Address to listen on for ssb websocket connections
wslis = ":8989" 

# Enable sending local UDP broadcasts
localadv = "on"
# Enable connecting to incoming UDP broadcasts
localdiscov = true
# Enable syncing by using epidemic-broadcast-trees (EBT)
enable-ebt = false 
# Bypass graph auth and fetch remote's feed, useful for pubs that are restoring their data from peers. Caveats abound, however.
promisc = false 
# Disable the UNIX socket RPC interface
nounixsock = false 

# how many feeds can be replicated with one peer connection using legacy gossip replication (shouldn't be higher than numRepl)
numPeer = 5
# how many feeds can be replicated concurrently using legacy gossip replication
numRepl = 10
`
	expectedConfig := SbotConfig{
		ShsCap:             "0KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=",
		Hops:               2,
		MuxRPCAddress:      ":8008",
		WebsocketAddress:   ":8989",
		EnableAdvertiseUDP: true,
		EnableDiscoveryUDP: true,
		EnableEBT:          false,
		EnableFirewall:     false,
		NoUnixSocket:       false,
		NumPeer:            5,
		NumRepl:            10,
	}
	testPath := filepath.Join(".", "testrun", t.Name())
	r.NoError(os.RemoveAll(testPath), "remove testrun folder")
	r.NoError(os.MkdirAll(testPath, 0700), "make new testrun folder")
	configPath := filepath.Join(testPath, "config.toml")
	err := os.WriteFile(configPath, []byte(configContents), 0700)
	r.NoError(err, "write config file")

	// now start an sbot with the config
	binName := "go-sbot-testing"
	binPath := filepath.Join(testPath, binName)

	goBuild := exec.Command("go", "build", "-o", binPath)
	goBuild.Stderr = os.Stderr
	goBuild.Stdout = os.Stderr
	err = goBuild.Run()
	r.NoError(err)

	bot1 := exec.Command(binPath, "-lis", ":0", "-repo", testPath)
	bot1.Stderr = os.Stderr
	bot1.Stdout = os.Stderr

	r.NoError(bot1.Start())
	// wait a bit to let it start properly, and read the config
	time.Sleep(5 * time.Second)
	out, err := exec.Command("kill", strconv.Itoa(bot1.Process.Pid)).CombinedOutput()
	r.NoError(err, "kill command failed: %s", string(out))
	err = bot1.Wait()
	r.NoError(err)

	// load the running config; the configuration the sbot was actually running with
	runningConfPath := filepath.Join(testPath, "running-config.json")
	b, err := os.ReadFile(runningConfPath)
	r.NoError(err)
	var runningConfig SbotConfig
	err = json.Unmarshal(b, &runningConfig)
	r.NoError(err)

	// test what we assume the config to have been against what it actually was
	r.EqualValues(expectedConfig.Hops, runningConfig.Hops)
	r.EqualValues(expectedConfig.MuxRPCAddress, runningConfig.MuxRPCAddress)
	r.EqualValues(expectedConfig.WebsocketAddress, runningConfig.WebsocketAddress)
	r.EqualValues(expectedConfig.EnableAdvertiseUDP, runningConfig.EnableAdvertiseUDP)
	r.EqualValues(expectedConfig.EnableDiscoveryUDP, runningConfig.EnableDiscoveryUDP)
	r.EqualValues(expectedConfig.EnableEBT, runningConfig.EnableEBT)
	r.EqualValues(expectedConfig.EnableFirewall, runningConfig.EnableFirewall)
	r.EqualValues(expectedConfig.NoUnixSocket, runningConfig.NoUnixSocket)
	r.EqualValues(expectedConfig.NumPeer, runningConfig.NumPeer)
	r.EqualValues(expectedConfig.NumRepl, runningConfig.NumRepl)
}
