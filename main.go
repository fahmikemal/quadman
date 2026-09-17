// Command quadman is a TUI manager for rootless Podman Quadlet units.
//
// quadman lists the Quadlet source files systemd's generator picks up,
// shows the runtime state of the units they generate, and lets you start,
// stop, restart, reload, and read journal logs — plus toggle user linger,
// the one setting every rootless container host needs.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/kemal-labs/quadman/internal/config"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/server"
	"github.com/kemal-labs/quadman/internal/systemd"
	"github.com/kemal-labs/quadman/internal/ui"
)

var version = "dev"

// quadletDirList is a repeatable --quadlet-dir flag.
type quadletDirList []string

func (q *quadletDirList) String() string { return strings.Join(*q, ",") }

func (q *quadletDirList) Set(v string) error {
	*q = append(*q, v)
	return nil
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	readonly := flag.Bool("readonly", false, "disable all state-changing actions (view, logs, and screens only)")
	sshTarget := flag.String("ssh", "", "run against a remote host over SSH (e.g. user@host); file edits are disabled in this mode")
	mouse := flag.Bool("mouse", false, "enable click-to-select (off by default so text selection keeps working)")
	theme := flag.String("theme", "", "color scheme: auto, dark, light, or colorblind (default from config.yaml)")
	var quadletDirs quadletDirList
	flag.Var(&quadletDirs, "quadlet-dir", "extra Quadlet source directory (repeatable; listed after the generator search path)")
	flag.Parse()

	if *showVersion {
		fmt.Println("quadman", moduleVersion())
		return
	}

	args := flag.Args()

	// Extra Quadlet source directories apply to every mode, including the
	// non-interactive list: config.yaml quadlet_dirs first, then
	// --quadlet-dir flags.
	applyQuadletDirs(quadletDirs)

	if len(args) > 0 {
		switch args[0] {
		case "list":
			list()
		case "version":
			fmt.Println("quadman", moduleVersion())
		case "serve":
			serve(args[1:], *readonly, *mouse, *theme, quadletDirs)
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q (available: list, serve, version)\n", args[0])
			os.Exit(2)
		}
		return
	}

	if *sshTarget != "" {
		if err := ui.RunWithOptions(ui.Options{SSH: *sshTarget, Readonly: *readonly, Mouse: *mouse, Theme: *theme, QuadletDirs: quadletDirs}); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if err := ui.RunWithOptions(ui.Options{Readonly: *readonly, Mouse: *mouse, Theme: *theme, QuadletDirs: quadletDirs}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// applyQuadletDirs appends user-configured Quadlet source directories to
// the discovery search path: quadlet_dirs from config.yaml first, then
// --quadlet-dir flags. A corrupt YAML file is reported; discovery still
// proceeds with flags only.
func applyQuadletDirs(flags []string) {
	seen := map[string]bool{}
	for _, d := range quadlet.ExtraDirs {
		seen[d] = true
	}
	add := func(dirs []string) {
		for _, d := range dirs {
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			quadlet.ExtraDirs = append(quadlet.ExtraDirs, d)
		}
	}
	if cfg, err := config.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "warning: config:", err)
	} else {
		add(cfg.Settings.QuadletDirs)
	}
	add(flags)
}

// moduleVersion reports the release version. Builds injected via ldflags
// fall back to the module version Go recorded at install time.
func moduleVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

// list prints a non-interactive overview of quadlet units and their state.
func list() {
	if err := runList(os.Stdout, systemd.New()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// runList renders the overview into w. It is separated from list() so tests
// can drive it with a fake systemctl and capture the output.
func runList(w io.Writer, sys *systemd.Systemd) error {
	units, err := quadlet.Discover()
	if err != nil {
		return err
	}
	if len(units) == 0 {
		fmt.Fprintln(w, "no quadlet units found in:")
		for _, d := range quadlet.SearchDirs() {
			fmt.Fprintln(w, "  "+d)
		}
		return nil
	}

	images := make([]string, len(units))
	for i := range units {
		info := quadlet.Inspect(units[i]) // resolves ServiceName= and image in one parse
		units[i].UnitName = info.UnitName
		images[i] = info.Image
	}

	if genDir := systemd.UserGeneratorDir(); genDir != "" {
		if stale := quadlet.StaleUnits(units, genDir); len(stale) > 0 {
			names := make([]string, 0, len(stale))
			for _, u := range stale {
				names = append(names, u.Name)
			}
			fmt.Fprintf(os.Stderr, "warning: %d quadlet file(s) changed since last daemon-reload: %s\n",
				len(stale), strings.Join(names, ", "))
			fmt.Fprintln(os.Stderr, "run: systemctl --user daemon-reload")
		}
	}

	statuses, serr := sys.Show(context.Background(), unitNames(units))
	if serr != nil {
		fmt.Fprintln(os.Stderr, "warning:", serr)
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "QUADLET\tKIND\tSYSTEMD UNIT\tSTATE\tSUB\tIMAGE")
	for i, u := range units {
		state, sub := "-", "-"
		if serr == nil {
			state, sub = statuses[u.UnitName].Display()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", u.Name, u.Kind, u.UnitName, state, sub, images[i])
	}
	return tw.Flush()
}

func unitNames(units []quadlet.Unit) []string {
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.UnitName)
	}
	return names
}

// serve starts the Wish SSH daemon so remote clients can run quadman over SSH.
func serve(args []string, defaultReadonly, defaultMouse bool, defaultTheme string, extraDirs []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "", "port to listen on (e.g. 2222, default :2222)")
	fs.StringVar(port, "p", "", "shorthand for --port")
	addr := fs.String("address", "", "listen address (e.g. :2222 or 0.0.0.0:2222)")
	fs.StringVar(addr, "a", "", "shorthand for --address")
	hostKey := fs.String("host-key", "", "path to private host key (default: auto-generated under ~/.config/quadman)")
	authKeys := fs.String("authorized-keys", "", "path to authorized_keys file (optional public key auth)")
	pass := fs.String("password", "", "optional password required to log in")
	readonly := fs.Bool("readonly", defaultReadonly, "disable all state-changing actions for connected sessions")
	mouse := fs.Bool("mouse", defaultMouse, "enable mouse support for connected sessions")
	theme := fs.String("theme", defaultTheme, "color scheme: auto, dark, light, or colorblind")
	banner := fs.String("banner", "", "custom SSH connection banner")
	idleTimeout := fs.Duration("idle-timeout", 0, "session idle timeout (default 30m)")
	maxTimeout := fs.Duration("max-timeout", 0, "max session duration (default 2h)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// Load settings from config.yaml if available
	cfg, _ := config.Load()
	scfg := cfg.Settings.Serve

	finalAddr := *addr
	if finalAddr == "" {
		if *port != "" {
			finalAddr = ":" + *port
		} else if scfg.Address != "" {
			finalAddr = scfg.Address
		} else if scfg.Port != "" {
			finalAddr = ":" + scfg.Port
		} else {
			finalAddr = server.DefaultAddress
		}
	}

	finalHostKey := *hostKey
	if finalHostKey == "" && scfg.HostKey != "" {
		finalHostKey = scfg.HostKey
	}

	finalAuthKeys := *authKeys
	if finalAuthKeys == "" && scfg.AuthorizedKeys != "" {
		finalAuthKeys = scfg.AuthorizedKeys
	}

	finalPass := *pass
	if finalPass == "" && scfg.Password != "" {
		finalPass = scfg.Password
	}

	finalReadonly := *readonly || scfg.Readonly

	opts := server.Options{
		Address:            finalAddr,
		HostKeyPath:        finalHostKey,
		AuthorizedKeysPath: finalAuthKeys,
		Password:           finalPass,
		Readonly:           finalReadonly,
		Mouse:              *mouse,
		Theme:              *theme,
		QuadletDirs:        extraDirs,
		Banner:             *banner,
		IdleTimeout:        *idleTimeout,
		MaxTimeout:         *maxTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("quadman SSH daemon starting on %s ...\n", opts.Address)
	fmt.Printf("Connect with: ssh %s\n", formatConnectHint(opts.Address))
	if finalReadonly {
		fmt.Println("Mode: readonly (all state-changing actions disabled)")
	}
	if finalAuthKeys != "" {
		fmt.Printf("Authentication: public keys from %s\n", finalAuthKeys)
	} else if finalPass != "" {
		fmt.Println("Authentication: password protected")
	} else {
		fmt.Println("Authentication: open (any public key accepted)")
	}
	fmt.Println("Press Ctrl+C to stop the server.")

	if err := server.Serve(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
	fmt.Println("\nquadman SSH daemon stopped gracefully.")
}

func formatConnectHint(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			port = strings.TrimPrefix(addr, ":")
		} else {
			return addr
		}
	}
	if port == "22" {
		return "<host>"
	}
	return "-p " + port + " <host>"
}
