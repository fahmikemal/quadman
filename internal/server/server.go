// Package server provides a native Wish SSH daemon to serve the quadman
// TUI directly over SSH without requiring quadman on the connecting client.
package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"

	"github.com/fahmikemal/quadman/internal/ui"
)

// DefaultHost is the loopback interface all implicit binds use. The daemon
// never listens beyond localhost unless an explicit address says so.
const DefaultHost = "127.0.0.1"

// DefaultAddress is the default loopback interface and port the server listens on.
const DefaultAddress = "127.0.0.1:2222"

// Options configures the SSH daemon.
type Options struct {
	// Address is the host:port to listen on (default 127.0.0.1:2222).
	// Non-loopback binds require AuthorizedKeysPath (see Validate).
	Address string

	// HostKeyPath is the file path of the server's private host key.
	// If the file does not exist, an ED25519 key pair is generated automatically.
	// Defaults to $XDG_CONFIG_HOME/quadman/host_ed25519.
	HostKeyPath string

	// AuthorizedKeysPath points to an OpenSSH authorized_keys file.
	// When non-empty, only public keys listed in this file are allowed.
	AuthorizedKeysPath string

	// Password sets an optional plaintext password requirement.
	Password string

	// Readonly forces all connecting sessions into readonly mode.
	Readonly bool

	// Mouse enables click-to-select and scrolling for connected sessions.
	Mouse bool

	// Theme selects the color scheme: auto, dark, light, or colorblind.
	Theme string

	// QuadletDirs lists extra Quadlet source directories to expose.
	QuadletDirs []string

	// IdleTimeout disconnects inactive sessions (default 30m).
	IdleTimeout time.Duration

	// MaxTimeout bounds the total session duration (default 2h).
	MaxTimeout time.Duration

	// Banner is displayed upon connection.
	Banner string

	// Version sets the SSH protocol version string.
	Version string

	// System targets system-wide (rootful) units instead of user units.
	System bool

	// Compartment serves another local user's Quadlet session through
	// non-interactive sudo (e.g. svc-web). Empty serves the operator's own
	// session. Served compartment sessions keep the isolated-session guards
	// (no file writes, no nested sudo hops).
	Compartment string
}

// defaultHostKeyPath returns the path to the auto-generated host key file.
func defaultHostKeyPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.Getenv("HOME")
		if dir == "" {
			dir = "."
		}
	}
	return filepath.Join(dir, "quadman", "host_ed25519")
}

// applyDefaults fills in default values for unset options.
func (o *Options) applyDefaults() {
	if o.Address == "" {
		o.Address = DefaultAddress
	}
	if o.HostKeyPath == "" {
		o.HostKeyPath = defaultHostKeyPath()
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 30 * time.Minute
	}
	if o.MaxTimeout <= 0 {
		o.MaxTimeout = 2 * time.Hour
	}
	if o.Banner == "" {
		o.Banner = "Welcome to quadman — rootless Quadlet manager\n\n"
	}
	if o.Version == "" {
		o.Version = "SSH-2.0-quadman"
	}
}

// IsLoopbackAddr reports whether addr binds loopback only (127/8, ::1, or
// localhost). Wildcards, LAN/public IPs, hostnames, and unparseable input
// all return false: fail closed.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// IsWildcardAddr reports whether addr binds all interfaces (":port",
// "0.0.0.0:port", "[::]:port"): reachable from the LAN, not just localhost.
func IsWildcardAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return strings.HasPrefix(addr, ":")
	}
	return host == "" || host == "0.0.0.0" || host == "::"
}

// Validate applies defaults and enforces the bind policy:
//
//   - open-auth (no authorized_keys and no password) always forces readonly,
//     even on loopback;
//   - any non-loopback bind requires an existing authorized_keys file, so
//     open-auth and password-only binds are loopback-only.
//
// Validation lives here — not just in the CLI — so every caller fails
// closed by construction.
func (o *Options) Validate() error {
	o.applyDefaults()

	if _, _, err := net.SplitHostPort(o.Address); err != nil {
		return fmt.Errorf("invalid address %q: %w", o.Address, err)
	}
	if o.AuthorizedKeysPath != "" {
		if err := checkAuthorizedKeys(o.AuthorizedKeysPath); err != nil {
			return err
		}
	}
	if o.AuthorizedKeysPath == "" && o.Password == "" {
		o.Readonly = true
	}
	if !IsLoopbackAddr(o.Address) && o.AuthorizedKeysPath == "" {
		_, port, _ := net.SplitHostPort(o.Address)
		hint := net.JoinHostPort(DefaultHost, port)
		if o.Password != "" {
			return fmt.Errorf("refusing password-only bind on non-loopback address %q: password auth alone is loopback-only; provide --authorized-keys or bind loopback (-a %s)", o.Address, hint)
		}
		return fmt.Errorf("refusing open-auth bind on non-loopback address %q: provide --authorized-keys or bind loopback (-a %s)", o.Address, hint)
	}
	return nil
}

// checkAuthorizedKeys rejects a missing, unreadable, or empty keys file so a
// typo fails loudly at startup instead of silently locking everyone out.
func checkAuthorizedKeys(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("authorized_keys %q: %w", path, err)
	}
	if st.IsDir() {
		return fmt.Errorf("authorized_keys %q is a directory", path)
	}
	if st.Size() == 0 {
		return fmt.Errorf("authorized_keys %q is empty", path)
	}
	return nil
}

// New creates and configures the Wish SSH server.
func New(opts Options) (*ssh.Server, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	// Ensure parent directory for the host key exists.
	if err := os.MkdirAll(filepath.Dir(opts.HostKeyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create host key directory: %w", err)
	}
	// An existing host key must never stay group/world-readable.
	if st, err := os.Stat(opts.HostKeyPath); err == nil && !st.IsDir() {
		if err := os.Chmod(opts.HostKeyPath, 0o600); err != nil {
			return nil, fmt.Errorf("secure host key permissions: %w", err)
		}
	}

	serverOpts := []ssh.Option{
		wish.WithAddress(opts.Address),
		wish.WithHostKeyPath(opts.HostKeyPath),
		wish.WithIdleTimeout(opts.IdleTimeout),
		wish.WithMaxTimeout(opts.MaxTimeout),
		wish.WithBanner(opts.Banner),
		wish.WithVersion(opts.Version),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler(opts)),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	}

	if opts.AuthorizedKeysPath != "" {
		serverOpts = append(serverOpts, wish.WithAuthorizedKeys(opts.AuthorizedKeysPath))
	}

	if opts.Password != "" {
		want := []byte(opts.Password)
		serverOpts = append(serverOpts, wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			return subtle.ConstantTimeCompare([]byte(password), want) == 1
		}))
	}

	return wish.NewServer(serverOpts...)
}

// uiOptions builds the TUI options for one incoming session.
func (opts Options) uiOptions(clientInfo string) ui.Options {
	return ui.Options{
		Readonly:    opts.Readonly,
		Mouse:       opts.Mouse,
		Theme:       opts.Theme,
		QuadletDirs: opts.QuadletDirs,
		ClientInfo:  clientInfo,
		NoEditor:    true,
		System:      opts.System,
		Compartment: opts.Compartment,
	}
}

// teaHandler returns the Bubble Tea handler for an incoming SSH session.
func teaHandler(opts Options) bubbletea.Handler {
	return func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
		clientInfo := sess.User()
		if ra := sess.RemoteAddr(); ra != nil {
			clientInfo += "@" + ra.String()
		}

		m := ui.NewWithOptions(opts.uiOptions(clientInfo))
		return m, []tea.ProgramOption{}
	}
}

// Serve starts the SSH server and blocks until the context is canceled or a
// fatal server error occurs. It performs a graceful shutdown when ctx is done.
func Serve(ctx context.Context, opts Options) error {
	srv, err := New(opts)
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		if lerr := srv.ListenAndServe(); lerr != nil && !errors.Is(lerr, ssh.ErrServerClosed) {
			errCh <- lerr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if serr := srv.Shutdown(shutdownCtx); serr != nil && !errors.Is(serr, ssh.ErrServerClosed) {
			return fmt.Errorf("shutdown: %w", serr)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// SplitHostPort parses host:port, :port, or a bare port into a valid listen address.
func SplitHostPort(addr, defaultPort string) (string, error) {
	if addr == "" {
		return ":" + defaultPort, nil
	}
	if !strings.Contains(addr, ":") {
		// Bare port or host
		if _, err := net.LookupPort("tcp", addr); err == nil {
			return ":" + addr, nil
		}
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if port == "" {
		port = defaultPort
	}
	return net.JoinHostPort(host, port), nil
}
