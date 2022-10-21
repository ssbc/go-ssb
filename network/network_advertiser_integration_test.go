// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// +build integration

package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ssbc/go-netwrap"

	"github.com/ssbc/go-ssb"
	multiserver "github.com/ssbc/go-ssb-multiserver"
)

// To ensure this works, add this to the SUDO file, where USER is your username.
/*
Defaults env_keep += "SEND_ADV RECV_ADV"
USER ALL=(ALL) NOPASSWD: /sbin/ip
USER ALL=(ALL) NOPASSWD: ! /sbin/ip netns exec *
USER ALL=(ALL) NOPASSWD: /sbin/ip netns exec local ip *
USER ALL=(ALL) NOPASSWD: /sbin/ip netns exec local sudo -u USER -- *
USER ALL=(ALL) NOPASSWD: /sbin/ip netns exec peer ip *
USER ALL=(ALL) NOPASSWD: /sbin/ip netns exec peer sudo -u USER -- *
USER ALL=(ALL) NOPASSWD: /sbin/ip netns exec testNet sudo -u USER -- *
USER ALL=(ALL) NOPASSWD: /usr/sbin/brctl
*/

func sudo(args ...string) *exec.Cmd {
	ret := exec.Command("sudo", args...)
	ret.Stderr = os.Stderr
	return ret
}

type NetConfig struct {
	Address   string
	Gateway   string
	Broadcast string
}

func createTestNetwork(name string, gateway NetConfig) error {
	// remove previous runs
	_ = sudo("ip", "link", "set", "dev", name, "down").Run()
	_ = sudo("brctl", "delbr", name).Run()

	err := sudo("brctl", "addbr", name).Run()
	if err != nil {
		return err
	}
	err = sudo("brctl", "stp", name, "off").Run()
	if err != nil {
		return fmt.Errorf("stp: %w", err)
	}
	err = sudo("ip", "link", "set", "dev", name, "up").Run()
	if err != nil {
		return fmt.Errorf("setting dev up: %w", err)
	}

	// Set gateway address
	if gateway.Address != "" {
		err = sudo("ip", "addr", "add", gateway.Address, "dev", name).Run()
		if err != nil {
			return fmt.Errorf("setting gateway address: %w", err)
		}
	}

	return nil
}

func addNode(nodeName string, netName string, addresses ...NetConfig) error {
	vethNode := nodeName
	vethHost := "br-" + nodeName

	bridgeName := netName
	netns := nodeName

	// wipe previous run
	_ = sudo("ip", "link", "del", vethHost).Run()
	_ = sudo("ip", "netns", "del", netns).Run()
	_ = sudo("brctl", "delif", bridgeName).Run()

	// make new
	err := sudo("ip", "link", "add",
		vethNode, "type", "veth", "peer", "name", vethHost,
	).Run()
	if err != nil {
		return fmt.Errorf("adding veth %s: %w", vethHost, err)
	}

	// Add to test net using bridge
	err = sudo("brctl", "addif", bridgeName, vethHost).Run()
	if err != nil {
		return fmt.Errorf("brctl addif: %w", err)
	}

	// Setup node's network namespace
	err = sudo("ip", "netns", "add", netns).Run()
	if err != nil {
		return fmt.Errorf("ip netns add: %w", err)
	}
	err = sudo("ip", "link", "set", vethNode, "netns", netns).Run()
	if err != nil {
		return fmt.Errorf("ip link set netns: %w", err)
	}

	// Bring up
	err = sudo("ip", "link", "set", "dev", vethHost, "up").Run()
	if err != nil {
		return fmt.Errorf("setting "+vethHost+" up: %w", err)
	}
	err = sudo("ip", "netns", "exec", netns, "ip", "link", "set", "dev", vethNode, "up").Run()
	if err != nil {
		return fmt.Errorf("setting "+vethNode+" up: %w", err)
	}

	// Set up IP addresses
	for _, address := range addresses {
		cmd := []string{"ip", "netns", "exec", netns, "ip", "addr", "add", address.Address}
		if address.Gateway != "" {
			cmd = append(cmd, "broadcast", address.Broadcast)
		}
		cmd = append(cmd, "dev", vethNode)
		err = sudo(cmd...).Run()
		err = fmt.Errorf("setting "+address.Address+" up: %w", err)
		if err != nil {
			return err
		}
		if address.Gateway != "" {
			err = sudo("ip", "netns", "exec", netns, "ip", "route", "add", "default", "via", address.Gateway).Run()
			err = fmt.Errorf("setting route up: %w", err)
			if err != nil {
				return err
			}
		}
	}

	return err
}

func assertSendAdvertisement(nodeName string, keyPair *ssb.KeyPair) (*exec.Cmd, error) {
	envVar := "SEND_ADV"
	value := bytes.NewBuffer(nil)
	err := json.NewEncoder(value).Encode(keyPair)
	if err != nil {
		return nil, fmt.Errorf("failed to encode keypair: %w", err)
	}

	user := os.Getenv("USER")
	cmd := sudo("ip", "netns", "exec", nodeName, "sudo", "-u", user, "--",
		os.Args[0], "-test.run=TestSendAdvertisement",
	)
	cmd.Env = append(os.Environ(), envVar+"="+string(value.String()))
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd, nil
}

func TestSendAdvertisement(t *testing.T) {
	sendAdv := os.Getenv("SEND_ADV")
	if sendAdv == "" {
		return
	}

	var keyPair ssb.KeyPair
	v := bytes.NewBufferString(sendAdv)
	err := json.NewDecoder(v).Decode(&keyPair)
	require.NoError(t, err)

	localAddr, err := net.ResolveUDPAddr("udp", "")
	require.NoError(t, err, "could not resolve listening address")

	adv, err := NewAdvertiser(localAddr, &keyPair)
	require.NoError(t, err, "could not send advertisement")

	adv.Start()
	require.NoError(t, err, "could not start advertiser")
	time.Sleep(15 * time.Second)
	adv.Stop()
}

func TestAdvertisementReceived(t *testing.T) {
	recvAdv := os.Getenv("RECV_ADV")
	if recvAdv == "" {
		return
	}

	wantAddr, err := multiserver.ParseNetAddress([]byte(recvAdv))
	require.NoError(t, err)

	kp := makeRandPubkey(t)
	disc, err := NewDiscoverer(kp)
	require.NoError(t, err)

	ch, cleanup := disc.Notify()

	addr, ok := <-ch
	if !ok {
		t.Fatal("rx ch closed")
		return
	}

	addrV := netwrap.GetAddr(addr, "tcp")
	require.NotNil(t, addrV, "wrong addr: %+v", addr)
	udpAddr, ok := addrV.(*net.TCPAddr)
	require.True(t, ok)

	require.True(t, udpAddr.IP.Equal(wantAddr.Addr.IP), "ips not equal. %s vs %s", udpAddr, wantAddr.Addr.IP)
	require.Equal(t, udpAddr.Port, wantAddr.Addr.Port)

	addrRef, err := ssb.GetFeedRefFromAddr(addr)
	require.NoError(t, err)
	require.Equal(t, wantAddr.Ref.ID, addrRef.ID)

	disc.Stop()
	cleanup()
}

func assertReceiveAdvertisement(t *testing.T, nodeName string, expected string) bool {
	envVar := "RECV_ADV"
	user := os.Getenv("USER")
	cmd := sudo("ip", "netns", "exec", nodeName, "sudo", "-u", user, "--",
		os.Args[0], "-test.run=TestAdvertisementReceived",
	)
	cmd.Env = append(os.Environ(), envVar+"="+expected)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	return assert.NoError(t, err, "did not receive advertisement")
}

func TestAdvertisementsOnIPSubnets(t *testing.T) {
	pubKey := makeTestPubKey(t)

	tests := []struct {
		Name                string
		Gateway             NetConfig
		Advertiser          []NetConfig
		AdvertiserPublicKey *ssb.KeyPair
		Peer                NetConfig
		PeerReceived        string
	}{
		{
			Name: "10.0.0.0/24",
			Gateway: NetConfig{
				Address:   "10.0.0.1",
				Broadcast: "10.255.255.255",
			},
			AdvertiserPublicKey: pubKey,
			Advertiser: []NetConfig{
				{
					Address:   "10.0.0.2/8",
					Gateway:   "10.0.0.1",
					Broadcast: "10.255.255.255",
				},
			},
			Peer: NetConfig{
				Address:   "10.0.0.3/8",
				Gateway:   "10.0.0.1",
				Broadcast: "10.255.255.255",
			},
			PeerReceived: "net:10.0.0.2:8008~shs:LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=",
		},
		{
			Name: "192.168.0.0 net:8",
			Gateway: NetConfig{
				Address:   "192.168.0.1",
				Broadcast: "192.168.255.255",
			},
			AdvertiserPublicKey: pubKey,
			Advertiser: []NetConfig{
				{
					Address:   "192.168.0.2/8",
					Gateway:   "192.168.0.1",
					Broadcast: "192.168.255.255",
				},
			},
			Peer: NetConfig{
				Address:   "192.168.0.3/8",
				Gateway:   "192.168.0.1",
				Broadcast: "192.168.255.255",
			},
			PeerReceived: "net:192.168.0.2:8008~shs:LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=",
		},
		{
			Name:                "IPv6 Standard",
			Gateway:             NetConfig{},
			AdvertiserPublicKey: pubKey,
			Advertiser: []NetConfig{
				{Address: "fe80::9e4e:36ff:feb3:bbbb/64"},
			},
			Peer: NetConfig{
				Address: "fe80::9e4e:36ff:feb3:cccc/64",
			},
			PeerReceived: "net:[fe80::9e4e:36ff:feb3:bbbb]:8008~shs:LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=",
		},
		// Broken: Wrong IP address received
		//		{
		//			Name: "Mixed Advertiser IPv4 + IPv6 -> IPv6",
		//			Gateway: NetConfig{
		//				Address:   "192.168.0.1/8",
		//				Broadcast: "192.168.255.255",
		//			},
		//			AdvertiserPublicKey: pubKey,
		//			Advertiser: []NetConfig{
		//				{
		//					Address:   "192.168.0.2/8",
		//					Gateway:   "192.168.0.1",
		//					Broadcast: "192.168.255.255",
		//				},
		//				{Address: "fe80::9e4e:36ff:feb3:bbbb/64"},
		//			},
		//			Peer: NetConfig{
		//				Address: "fe80::9e4e:36ff:feb3:cccc/64",
		//			},
		//			PeerReceived: "net:[fe80::9e4e:36ff:feb3:bbbb]:8008~shs:LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=",
		//		},
		{
			Name: "Mixed Advertiser IPv4 + IPv6 -> IPv4",
			Gateway: NetConfig{
				Address:   "192.168.0.1/8",
				Broadcast: "192.168.255.255",
			},
			AdvertiserPublicKey: pubKey,
			Advertiser: []NetConfig{
				{
					Address:   "192.168.0.2/8",
					Gateway:   "192.168.0.1",
					Broadcast: "192.168.255.255",
				},
				{Address: "fe80::9e4e:36ff:feb3:bbbb/64"},
			},
			Peer: NetConfig{
				Address:   "192.168.0.3/8",
				Gateway:   "192.168.0.1",
				Broadcast: "192.168.255.255",
			},
			PeerReceived: "net:192.168.0.2:8008~shs:LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=",
		},
	}

	testNet := "testNet"

	for _, test := range tests {
		fmt.Println(test.Name, "starting")
		// Construct test network
		err := createTestNetwork(testNet, test.Gateway)
		require.NoError(t, err)

		err = addNode("local", testNet, test.Advertiser...)
		require.NoError(t, err)

		err = addNode("peer", testNet, test.Peer)
		require.NoError(t, err)

		txCmd, err := assertSendAdvertisement("local", test.AdvertiserPublicKey)
		require.NoError(t, err)
		done := make(chan struct{})
		go func(n string) {

			if err := txCmd.Start(); err != nil {
				fmt.Println(n, "- txcmd not started", err)
			}
			done <- struct{}{}

			fmt.Println(n, "txcmd running")

			if err := txCmd.Wait(); err != nil {
				fmt.Println(n, " - txcmd wait failed", err)
			}
			fmt.Println(n, "txcmd done")
			close(done)
		}(test.Name)

		<-done

		assertReceiveAdvertisement(t, "peer", test.PeerReceived)

		fmt.Println(test.Name, "received")
		// thats only the ip netns exec ..
		// txCmd.Process.Signal(os.Interrupt)
		<-done
	}
}
