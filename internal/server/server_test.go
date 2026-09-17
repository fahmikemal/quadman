package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cryptossh "golang.org/x/crypto/ssh"
)

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		input       string
		defaultPort string
		want        string
		wantErr     bool
	}{
		{"", "2222", ":2222", false},
		{"2222", "8080", ":2222", false},
		{":2222", "8080", ":2222", false},
		{"127.0.0.1:2222", "8080", "127.0.0.1:2222", false},
		{"0.0.0.0:2222", "8080", "0.0.0.0:2222", false},
		{"invalid::addr", "2222", "", true},
	}

	for _, tc := range tests {
		got, err := SplitHostPort(tc.input, tc.defaultPort)
		if (err != nil) != tc.wantErr {
			t.Errorf("SplitHostPort(%q, %q) error = %v, wantErr %v", tc.input, tc.defaultPort, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("SplitHostPort(%q, %q) = %q, want %q", tc.input, tc.defaultPort, got, tc.want)
		}
	}
}

func TestNewServer(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_host_key")

	opts := Options{
		Address:     "127.0.0.1:0", // ephemeral port
		HostKeyPath: keyPath,
		Readonly:    true,
		Mouse:       true,
		Password:    "secret123",
	}

	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if srv == nil {
		t.Fatal("New() returned nil server")
	}

	if srv.Addr != "127.0.0.1:0" {
		t.Errorf("srv.Addr = %q, want %q", srv.Addr, "127.0.0.1:0")
	}
}

func TestServeGracefulShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_host_key")

	// Choose a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	addr := l.Addr().String()
	l.Close() // release so server can bind

	opts := Options{
		Address:     addr,
		HostKeyPath: keyPath,
		Readonly:    true,
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, opts)
	}()

	// Give server time to bind and begin listening
	time.Sleep(200 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve() returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() failed to shut down within timeout")
	}
}

func TestSSHClientInteractiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_host_key")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	addr := l.Addr().String()
	l.Close()

	opts := Options{
		Address:     addr,
		HostKeyPath: keyPath,
		Readonly:    true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = Serve(ctx, opts)
	}()

	// Wait for server to accept connections
	time.Sleep(200 * time.Millisecond)

	_, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := cryptossh.NewSignerFromKey(clientKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	clientConfig := &cryptossh.ClientConfig{
		User: "testuser",
		Auth: []cryptossh.AuthMethod{
			cryptossh.PublicKeys(signer),
		},
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(), // #nosec G106 -- test only
		Timeout:         5 * time.Second,
	}

	client, err := cryptossh.Dial("tcp", addr, clientConfig)
	if err != nil {
		t.Fatalf("ssh.Dial error: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession error: %v", err)
	}
	defer session.Close()

	// Request a PTY so activeterm middleware passes
	modes := cryptossh.TerminalModes{
		cryptossh.ECHO:          0,
		cryptossh.TTY_OP_ISPEED: 14400,
		cryptossh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 80, 24, modes); err != nil {
		t.Fatalf("RequestPty error: %v", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe error: %v", err)
	}

	var stdout bytes.Buffer
	session.Stdout = &stdout

	if err := session.Shell(); err != nil {
		t.Fatalf("session.Shell error: %v", err)
	}

	// Wait briefly for the UI to render
	time.Sleep(400 * time.Millisecond)

	// Send 'q' to exit the Bubble Tea program cleanly
	_, _ = stdin.Write([]byte("q"))

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case <-done:
		// Program exited cleanly
	case <-time.After(3 * time.Second):
		t.Fatal("session did not exit after sending 'q'")
	}

	if !strings.Contains(stdout.String(), "quadman") {
		t.Errorf("stdout does not contain 'quadman': got %q", stdout.String())
	}
}
