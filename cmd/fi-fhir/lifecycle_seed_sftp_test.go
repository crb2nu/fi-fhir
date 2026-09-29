package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// seedSFTPServer is an in-process SSH/SFTP server over the real filesystem,
// the same shape as the batch proof's startBatchSFTPServer. It lets the seed's
// real connection validation run without a network or a container.
type seedSFTPServer struct {
	root       string
	address    string
	host       string
	port       int
	knownHosts string
	listener   net.Listener
	wait       sync.WaitGroup
}

const (
	seedSFTPUser     = "seed-user"
	seedSFTPPassword = "seed-pass"
)

func startSeedSFTPServer(t *testing.T) *seedSFTPServer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &seedSFTPServer{root: t.TempDir(), address: listener.Addr().String(), listener: listener}
	host, portText, err := net.SplitHostPort(server.address)
	if err != nil {
		t.Fatal(err)
	}
	server.host = host
	server.port, _ = strconv.Atoi(portText)
	server.knownHosts = writeSeedKnownHosts(t, server.address, signer.PublicKey())
	config := &ssh.ServerConfig{PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if metadata.User() != seedSFTPUser || string(password) != seedSFTPPassword {
			return nil, errors.New("denied")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	server.wait.Add(1)
	go func() {
		defer server.wait.Done()
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			server.wait.Add(1)
			go func() {
				defer server.wait.Done()
				serveSeedSFTPConnection(connection, config)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		server.wait.Wait()
	})
	return server
}

func serveSeedSFTPConnection(connection net.Conn, config *ssh.ServerConfig) {
	sshConnection, channels, requests, err := ssh.NewServerConn(connection, config)
	if err != nil {
		_ = connection.Close()
		return
	}
	defer func() { _ = sshConnection.Close() }()
	go ssh.DiscardRequests(requests)
	for channelRequest := range channels {
		if channelRequest.ChannelType() != "session" {
			_ = channelRequest.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, channelRequests, err := channelRequest.Accept()
		if err != nil {
			continue
		}
		for request := range channelRequests {
			accepted := request.Type == "subsystem" && len(request.Payload) >= 4 && string(request.Payload[4:]) == "sftp"
			_ = request.Reply(accepted, nil)
			if !accepted {
				continue
			}
			server, err := sftp.NewServer(channel)
			if err == nil {
				_ = server.Serve()
				_ = server.Close()
			}
			break
		}
	}
}

func writeSeedKnownHosts(t *testing.T, address string, key ssh.PublicKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, key) + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// wrongSeedKnownHosts pins a different key for the server's address.
func wrongSeedKnownHosts(t *testing.T, address string) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return writeSeedKnownHosts(t, address, signer.PublicKey())
}

// startSilentListener accepts TCP connections and never speaks, so an SSH
// handshake against it only ends when the caller gives up.
func startSilentListener(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var connections []net.Conn
	var mutex sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			mutex.Lock()
			connections = append(connections, connection)
			mutex.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
		mutex.Lock()
		defer mutex.Unlock()
		for _, connection := range connections {
			_ = connection.Close()
		}
	})
	return listener.Addr().String()
}
