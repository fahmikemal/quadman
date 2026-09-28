package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
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

func TestDefaultAddressIsLoopback(t *testing.T) {
	if !IsLoopbackAddr(DefaultAddress) {
		t.Errorf("DefaultAddress = %q, must bind loopback only", DefaultAddress)
	}
	if IsWildcardAddr(DefaultAddress) {
		t.Errorf("DefaultAddress = %q, must not be a wildcard bind", DefaultAddress)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:2222", true},
		{"127.0.0.2:2222", true},
		{"[::1]:2222", true},
		{"localhost:2222", true},
		{":2222", false},
		{"0.0.0.0:2222", false},
		{"[::]:2222", false},
		{"192.168.1.5:2222", false},
		{"example.com:2222", false},
		{"", false},
		{"not-an-addr", false},
	}
	for _, tc := range tests {
		if got := IsLoopbackAddr(tc.addr); got != tc.want {
			t.Errorf("IsLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestIsWildcardAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{":2222", true},
		{"0.0.0.0:2222", true},
		{"[::]:2222", true},
		{"127.0.0.1:2222", false},
		{"[::1]:2222", false},
		{"192.168.1.1:22", false},
	}
	for _, tc := range tests {
		if got := IsWildcardAddr(tc.addr); got != tc.want {
			t.Errorf("IsWildcardAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func writeAuthKeys(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(path, []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMtest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateBindPolicy(t *testing.T) {
	t.Run("loopback open-auth allowed and forced readonly", func(t *testing.T) {
		opts := Options{Address: "127.0.0.1:2222", HostKeyPath: filepath.Join(t.TempDir(), "k")}
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if !opts.Readonly {
			t.Error("open-auth on loopback must force Readonly")
		}
	})
	t.Run("loopback password-only allowed", func(t *testing.T) {
		opts := Options{Address: "127.0.0.1:2222", HostKeyPath: filepath.Join(t.TempDir(), "k"), Password: "secret123"}
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
	t.Run("wildcard open-auth refused", func(t *testing.T) {
		opts := Options{Address: ":2222", HostKeyPath: filepath.Join(t.TempDir(), "k")}
		if err := opts.Validate(); err == nil || !strings.Contains(err.Error(), "authorized-keys") {
			t.Errorf("wildcard open-auth must demand authorized-keys, got %v", err)
		}
	})
	t.Run("wildcard password-only refused", func(t *testing.T) {
		opts := Options{Address: "0.0.0.0:2222", HostKeyPath: filepath.Join(t.TempDir(), "k"), Password: "secret123"}
		if err := opts.Validate(); err == nil || !strings.Contains(err.Error(), "authorized-keys") {
			t.Errorf("wildcard password-only must demand authorized-keys, got %v", err)
		}
	})
	t.Run("LAN open-auth refused", func(t *testing.T) {
		opts := Options{Address: "192.168.1.5:2222", HostKeyPath: filepath.Join(t.TempDir(), "k")}
		if err := opts.Validate(); err == nil {
			t.Error("LAN bind without authorized-keys must fail")
		}
	})
	t.Run("wildcard with keys allowed", func(t *testing.T) {
		opts := Options{Address: ":2222", HostKeyPath: filepath.Join(t.TempDir(), "k"), AuthorizedKeysPath: writeAuthKeys(t)}
		if err := opts.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
	t.Run("missing keys file refused", func(t *testing.T) {
		opts := Options{Address: "127.0.0.1:2222", HostKeyPath: filepath.Join(t.TempDir(), "k"), AuthorizedKeysPath: filepath.Join(t.TempDir(), "nope")}
		if err := opts.Validate(); err == nil {
			t.Error("missing authorized_keys file must fail")
		}
	})
}

func TestUIOptionsPassthrough(t *testing.T) {
	opts := Options{
		Address:     "127.0.0.1:2222",
		HostKeyPath: filepath.Join(t.TempDir(), "k"),
		Readonly:    true,
		Mouse:       true,
		Theme:       "dark",
		QuadletDirs: []string{"/tmp/q"},
		System:      true,
		Compartment: "svc-web",
	}
	u := opts.uiOptions("tester@127.0.0.1:1")
	if !u.Readonly || !u.Mouse || u.Theme != "dark" || !u.System || !u.NoEditor {
		t.Errorf("session flags lost: %+v", u)
	}
	if u.Compartment != "svc-web" {
		t.Errorf("Compartment = %q, want svc-web", u.Compartment)
	}
	if u.ClientInfo != "tester@127.0.0.1:1" || len(u.QuadletDirs) != 1 {
		t.Errorf("client info or dirs lost: %+v", u)
	}
}

func TestNewRejectsPublicOpenAuth(t *testing.T) {
	opts := Options{Address: ":2222", HostKeyPath: filepath.Join(t.TempDir(), "k")}
	if _, err := New(opts); err == nil {
		t.Error("New() must refuse wildcard bind without authorized-keys")
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
