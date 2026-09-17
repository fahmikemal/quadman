// Package server provides a native Wish SSH daemon to serve the quadman
// TUI directly over SSH without requiring quadman on the connecting client.
package server

import (
	"context"
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

	"github.com/kemal-labs/quadman/internal/ui"
)

// DefaultAddress is the default network interface and port the server listens on.
const DefaultAddress = ":2222"

// Options configures the SSH daemon.
type Options struct {
	// Address is the host:port or :port to listen on (default :2222).
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

// New creates and configures the Wish SSH server.
func New(opts Options) (*ssh.Server, error) {
	opts.applyDefaults()

	// Ensure parent directory for the host key exists.
	if err := os.MkdirAll(filepath.Dir(opts.HostKeyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create host key directory: %w", err)
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
		serverOpts = append(serverOpts, wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			return password == opts.Password
		}))
	}

	return wish.NewServer(serverOpts...)
}

// teaHandler returns the Bubble Tea handler for an incoming SSH session.
func teaHandler(opts Options) bubbletea.Handler {
	return func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
		clientInfo := sess.User()
		if ra := sess.RemoteAddr(); ra != nil {
			clientInfo += "@" + ra.String()
		}

		uiOpts := ui.Options{
			Readonly:    opts.Readonly,
			Mouse:       opts.Mouse,
			Theme:       opts.Theme,
			QuadletDirs: opts.QuadletDirs,
			ClientInfo:  clientInfo,
			NoEditor:    true,
			System:      opts.System,
		}

		m := ui.NewWithOptions(uiOpts)
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
