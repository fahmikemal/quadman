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
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/fahmikemal/quadman/internal/compartment"
	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/server"
	"github.com/fahmikemal/quadman/internal/skill"
	"github.com/fahmikemal/quadman/internal/systemd"
	"github.com/fahmikemal/quadman/internal/ui"
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
	asUser := flag.String("as", "", "manage another local user's Quadlet units via sudo (compartment, e.g. --as svc-web); file edits are disabled in this mode")
	mouse := flag.Bool("mouse", false, "enable click-to-select (off by default so text selection keeps working)")
	theme := flag.String("theme", "", "color scheme: auto, dark, light, or colorblind (default from config.yaml)")
	ascii := flag.Bool("ascii", false, "ASCII status glyphs instead of Unicode shapes (automatic on TERM=linux)")
	systemFlag := flag.Bool("system", false, "manage system-wide (rootful) Quadlet units instead of user units")
	showSkill := flag.Bool("skill", false, "export AI agent skill definition (markdown or json) and exit")
	skillFormat := flag.String("skill-format", "markdown", "output format for --skill: markdown or json")
	var quadletDirs quadletDirList
	flag.Var(&quadletDirs, "quadlet-dir", "extra Quadlet source directory (repeatable; listed after the generator search path)")
	flag.Parse()

	if *showVersion {
		printVersion()
		return
	}

	if *showSkill {
		printSkill(*skillFormat)
		return
	}

	// Extra Quadlet source directories apply to every mode, including the
	// non-interactive list: config.yaml quadlet_dirs first, then
	// --quadlet-dir flags.
	cfg, cfgErr := config.Load()
	applyQuadletDirs(cfg, cfgErr, quadletDirs)

	system, err := resolveMode(*systemFlag, *asUser, *sshTarget, cfg.System(), os.Getuid())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	// Multiplexed SSH masters live for a minute after the last call; close
	// ours on the way out so nothing outlives the process.
	defer remote.Shutdown(context.Background())

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "list":
			rejectMisplaced("list", args[1:])
			if err := listCmd(cfg, system, *asUser, *sshTarget); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				remote.Shutdown(context.Background())
				os.Exit(1)
			}
		case "version":
			rejectMisplaced("version", args[1:])
			printVersion()
		case "serve":
			if *sshTarget != "" {
				fmt.Fprintln(os.Stderr, "error: serve runs on this host and cannot be combined with --ssh (run quadman serve on the remote host instead)")
				os.Exit(2)
			}
			serve(args[1:], *readonly, *mouse, *theme, *ascii, quadletDirs, system, *asUser, cfg, cfgErr)
		case "skill":
			rejectMisplaced("skill", args[1:], "format", "json")
			format := "markdown"
			for i := 1; i < len(args); i++ {
				arg := args[i]
				if arg == "json" || arg == "--json" || arg == "-json" {
					format = "json"
				} else if strings.HasPrefix(arg, "--format=") || strings.HasPrefix(arg, "-format=") {
					format = strings.SplitN(arg, "=", 2)[1]
				} else if (arg == "--format" || arg == "-format") && i+1 < len(args) {
					format = args[i+1]
					i++
				}
			}
			printSkill(format)
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q (available: list, serve, skill, version)\n", args[0])
			os.Exit(2)
		}
		return
	}

	// A compartment that cannot be reached must stop here, like list and
	// serve: opening the TUI on the operator's own session instead would
	// invite actions on the wrong user's units.
	if *asUser != "" {
		if _, err := compSetup(*asUser); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
	opts := ui.Options{
		SSH: *sshTarget, Compartment: *asUser, Readonly: *readonly, Mouse: *mouse,
		Theme: *theme, ASCII: *ascii, QuadletDirs: quadletDirs, System: system,
	}
	if err := ui.RunWithOptions(opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		remote.Shutdown(context.Background())
		os.Exit(1)
	}
}

// resolveMode decides whether the session targets the system instance,
// from the flags, config.yaml's `system:` default, and the effective uid.
// Combinations quadman cannot honor are rejected instead of silently
// dropping one of the options:
//
//   - --system with --as: system units live outside any user's session;
//   - --ssh with --as: no nested sudo hop over SSH (run --as on the host).
//
// A compartment (--as) is always a user session, even for a root operator,
// so neither the uid nor config.yaml turns it into system mode. Running as
// root locally selects system mode, like `sudo quadman`; over --ssh only
// the flag or config.yaml do (the local uid says nothing about the host).
func resolveMode(systemFlag bool, asUser, sshTarget string, cfgSystem bool, uid int) (bool, error) {
	if err := checkModeConflict(systemFlag, asUser); err != nil {
		return false, err
	}
	switch {
	case sshTarget != "" && asUser != "":
		return false, fmt.Errorf("--ssh and --as cannot be combined (no nested sudo over SSH): run quadman --as %s on that host", asUser)
	case asUser != "":
		return false, nil
	case systemFlag:
		return true, nil
	case sshTarget != "":
		return cfgSystem, nil
	}
	return cfgSystem || uid == 0, nil
}

// checkModeConflict rejects --system combined with --as: system units live
// outside any user's session, so the pair would mix system discovery with a
// compartment runner. Callers exit 2 on error.
func checkModeConflict(system bool, asUser string) error {
	if system && asUser != "" {
		return fmt.Errorf("--system and --as cannot be combined (system units live outside any user's session)")
	}
	return nil
}

// misplacedFlag returns the name of the first flag found after the
// subcommand, where Go's flag package would silently ignore it (list takes
// no flags of its own, so any flag there is a dropped global option).
// Empty means no misplaced flag. Callers reject this explicitly instead of
// running with silently dropped options.
func misplacedFlag(args []string) string {
	return misplacedFlagExcept(args)
}

// misplacedFlagExcept is misplacedFlag with an allowlist for subcommands
// that take their own flags (skill --format/--json).
func misplacedFlagExcept(args []string, allowed ...string) string {
	allow := map[string]bool{}
	for _, a := range allowed {
		allow[a] = true
	}
	for _, a := range args {
		if a == "--" {
			return ""
		}
		if len(a) > 1 && a[0] == '-' {
			name := strings.TrimLeft(a, "-")
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
			if !allow[name] {
				return name
			}
		}
	}
	return ""
}

// rejectMisplaced exits 2 when a global flag trails the subcommand, where
// it would be silently ignored. Allowed names pass through for subcommands
// with their own flags.
func rejectMisplaced(cmd string, args []string, allowed ...string) {
	if bad := misplacedFlagExcept(args, allowed...); bad != "" {
		fmt.Fprintf(os.Stderr, "error: place --%s before the command (quadman --%s ... %s)\n", bad, bad, cmd)
		os.Exit(2)
	}
}

// applyQuadletDirs appends user-configured Quadlet source directories to
// the discovery search path: quadlet_dirs from config.yaml first, then
// --quadlet-dir flags. A config problem is reported once here; discovery
// still proceeds with what is known.
func applyQuadletDirs(cfg config.Config, cfgErr error, flags []string) {
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "warning: config:", cfgErr)
	}
	quadlet.AddExtraDirs(cfg.Settings.QuadletDirs)
	quadlet.AddExtraDirs(flags)
}

// printVersion prints the block-art splash on a terminal and the plain
// "quadman <version>" line otherwise, so scripts that read the version
// keep working.
func printVersion() {
	if fi, err := os.Stdout.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		fmt.Println("quadman", moduleVersion())
		return
	}
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		width = 80
	}
	_, _ = lipgloss.Println(ui.Splash(moduleVersion(), versionInfo(), width))
}

// versionInfo is the splash's runtime line: podman, systemd, Go, commit.
func versionInfo() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var parts []string
	if v, err := (podman.Client{}).Version(ctx); err == nil && v != "" {
		parts = append(parts, "podman "+v)
	}
	if out, err := exec.CommandContext(ctx, "systemctl", "--version").Output(); err == nil {
		if f := strings.Fields(string(out)); len(f) >= 2 {
			parts = append(parts, f[0]+" "+f[1])
		}
	}
	parts = append(parts, runtime.Version()+" "+runtime.GOOS+"/"+runtime.GOARCH)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				parts = append(parts, "commit "+s.Value[:7])
			}
		}
	}
	return strings.Join(parts, " · ")
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

// listCmd prints a non-interactive overview of quadlet units and their
// state, for the local session, a compartment (--as), or an SSH host
// (--ssh) — the same targets as the TUI, never silently the wrong one.
func listCmd(cfg config.Config, system bool, asUser, sshTarget string) error {
	sys := systemd.New()
	if system {
		sys = systemd.NewSystem()
	}
	dirs := quadlet.SearchDirsMode(system)
	switch {
	case sshTarget != "":
		sys.Remote = remote.Runner{Target: sshTarget, Multiplex: cfg.SSHMultiplex()}
		dirs = nil
	case asUser != "":
		c, err := compSetup(asUser)
		if err != nil {
			return err
		}
		sys.Remote = c.Runner()
		sys.GenDir = c.GeneratorDir()
		dirs = quadlet.SearchDirsFor(c.Home, c.UID, c.RuntimeDir)
	}
	return runList(os.Stdout, sys, system, dirs)
}

// compSetup probes, resolves, and runtime-checks a compartment user for
// non-interactive commands, sharing the TUI's all-or-nothing rule: any
// failure aborts before anything runs.
func compSetup(username string) (compartment.Compartment, error) {
	return compartment.Setup(context.Background(), username)
}

// runList renders the overview into w. It is separated from listCmd() so
// tests can drive it with a fake systemctl and capture the output. Stale
// detection comes from systemd's own NeedDaemonReload, so it is right for
// local, compartment, and SSH sessions alike.
func runList(w io.Writer, sys *systemd.Systemd, system bool, dirs []string) error {
	ctx := context.Background()
	var units []quadlet.Unit
	var images []string
	var err error
	if sys.Remote.Isolated() {
		// Compartments/SSH: enumerate through the session runner
		// (`podman quadlet list`, generated-unit fallback), never the
		// operator's own filesystem.
		units, err = quadlet.DiscoverRemoteMode(ctx, sys.Remote, system)
		if err != nil {
			return err
		}
		images = make([]string, len(units))
		for i := range units {
			info := quadlet.InspectRemote(ctx, sys.Remote, units[i])
			if units[i].Kind != quadlet.KindQuadlets {
				units[i].UnitName = info.UnitName
			}
			images[i] = info.Image
		}
	} else {
		units, err = quadlet.DiscoverDirs(dirs)
		if err != nil {
			return err
		}
		images = make([]string, len(units))
		for i := range units {
			info := quadlet.Inspect(units[i]) // resolves ServiceName= and image in one parse
			if units[i].Kind != quadlet.KindQuadlets {
				units[i].UnitName = info.UnitName
			}
			images[i] = info.Image
		}
	}

	if len(units) == 0 {
		if label := sys.Remote.Label(); label != "" {
			_, err := fmt.Fprintln(w, "no quadlet units found on "+label)
			return err
		}
		if _, err := fmt.Fprintln(w, "no quadlet units found in:"); err != nil {
			return err
		}
		for _, d := range dirs {
			if _, err := fmt.Fprintln(w, "  "+d); err != nil {
				return err
			}
		}
		return nil
	}

	statuses, serr := sys.Show(ctx, unitNames(units))
	if serr != nil {
		_, _ = fmt.Fprintln(os.Stderr, "warning:", serr)
	} else if stale := staleNames(units, statuses); len(stale) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "warning: %d quadlet file(s) changed since last daemon-reload: %s\n",
			len(stale), strings.Join(stale, ", "))
		_, _ = fmt.Fprintln(os.Stderr, "run: "+reloadHint(sys, system))
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "QUADLET\tKIND\tSYSTEMD UNIT\tSTATE\tSUB\tIMAGE")
	for i, u := range units {
		state, sub := "-", "-"
		if serr == nil && u.UnitName != "" {
			state, sub = statuses[u.UnitName].Display()
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", u.Name, u.Kind, orDash(u.UnitName), state, sub, orDash(images[i]))
	}
	return tw.Flush()
}

// staleNames lists the units a daemon-reload would change (extra-dir
// units excepted: the generator never reads them).
func staleNames(units []quadlet.Unit, statuses map[string]systemd.Status) []string {
	var names []string
	extra := quadlet.ExtraDirList()
	for _, u := range units {
		if u.UnitName == "" || inDirs(u.Path, extra) {
			continue
		}
		if st, ok := statuses[u.UnitName]; ok && st.NeedsReload() {
			names = append(names, u.Name)
		}
	}
	return names
}

func inDirs(path string, dirs []string) bool {
	for _, d := range dirs {
		rel, err := filepath.Rel(filepath.Clean(d), filepath.Clean(path))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// reloadHint is the exact daemon-reload command for the session.
func reloadHint(sys *systemd.Systemd, system bool) string {
	cmd := "systemctl --user daemon-reload"
	if system {
		cmd = "systemctl daemon-reload"
	}
	switch {
	case sys.Remote.IsRemote():
		return "ssh " + sys.Remote.Target + " " + cmd
	case sys.Remote.As != "":
		return "sudo -u " + sys.Remote.As + " XDG_RUNTIME_DIR=/run/user/$(id -u " + sys.Remote.As + ") " + cmd
	}
	return cmd
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func unitNames(units []quadlet.Unit) []string {
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.UnitName)
	}
	return names
}

// serve starts the Wish SSH daemon so remote clients can run quadman over SSH.
func serve(args []string, defaultReadonly, defaultMouse bool, defaultTheme string, ascii bool, extraDirs []string, system bool, asUser string, cfg config.Config, cfgErr error) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "", "port to listen on loopback (e.g. 2222, default 127.0.0.1:2222)")
	fs.StringVar(port, "p", "", "shorthand for --port")
	addr := fs.String("address", "", "listen address (default 127.0.0.1:2222; use -a 0.0.0.0:2222 with --authorized-keys for LAN)")
	fs.StringVar(addr, "a", "", "shorthand for --address")
	hostKey := fs.String("host-key", "", "path to private host key (default: auto-generated under ~/.config/quadman)")
	authKeys := fs.String("authorized-keys", "", "path to authorized_keys file (optional public key auth)")
	pass := fs.String("password", "", "optional password required to log in (visible in the process list; prefer --password-file)")
	passFile := fs.String("password-file", "", "read the login password from a file instead of --password")
	readonly := fs.Bool("readonly", defaultReadonly, "disable all state-changing actions for connected sessions")
	mouse := fs.Bool("mouse", defaultMouse, "enable mouse support for connected sessions")
	if defaultTheme == "" {
		defaultTheme = cfg.Settings.Theme
	}
	theme := fs.String("theme", defaultTheme, "color scheme: dark, light, or colorblind (auto resolves to dark: the daemon has no terminal to probe)")
	banner := fs.String("banner", "", "custom SSH connection banner")
	idleTimeout := fs.Duration("idle-timeout", 0, "session idle timeout (default 30m)")
	maxTimeout := fs.Duration("max-timeout", 0, "max session duration (default 2h)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// A broken compartment must fail the daemon, never silently serve the
	// operator's own session to clients expecting someone else's.
	if asUser != "" {
		if _, err := compSetup(asUser); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}

	// A daemon that cannot read its settings must not guess: a dropped
	// `readonly: true` or authorized_keys would change who may do what.
	if config.IsSettingsError(cfgErr) {
		fmt.Fprintln(os.Stderr, "error: refusing to serve with an unusable config.yaml:", cfgErr)
		os.Exit(2)
	}
	scfg := cfg.Settings.Serve

	finalPass, passSource, passWarn, passErr := resolveServePassword(*pass, *passFile, scfg, os.Getenv)
	if passErr != nil {
		fmt.Fprintln(os.Stderr, "error:", passErr)
		os.Exit(2)
	}
	if passWarn != "" {
		fmt.Fprintln(os.Stderr, passWarn)
	}
	if passSource == "config password" {
		if yp, err := config.YAMLPath(); err == nil {
			if st, err := os.Stat(yp); err == nil && st.Mode().Perm()&0o077 != 0 {
				fmt.Fprintf(os.Stderr, "WARNING: %s holds a password and is readable beyond owner (mode %04o); run: chmod 600 %s\n", yp, st.Mode().Perm(), yp)
			}
		}
	}

	finalAddr := resolveServeAddr(*addr, *port, scfg)

	finalHostKey := *hostKey
	if finalHostKey == "" && scfg.HostKey != "" {
		finalHostKey = scfg.HostKey
	}

	finalAuthKeys := *authKeys
	if finalAuthKeys == "" && scfg.AuthorizedKeys != "" {
		finalAuthKeys = scfg.AuthorizedKeys
	}

	openAuth := finalAuthKeys == "" && finalPass == ""
	finalReadonly := *readonly || scfg.Readonly || openAuth

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
		System:             system,
		Compartment:        asUser,
		ASCII:              ascii,
	}

	// Fail fast on insecure binds (non-loopback without authorized_keys)
	// before printing the startup banner.
	if err := opts.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("quadman SSH daemon starting on %s ...\n", opts.Address)
	fmt.Printf("Connect with: ssh %s\n", formatConnectHint(opts.Address))
	if asUser != "" {
		fmt.Printf("Compartment: %s (non-interactive sudo; isolated-session guards apply)\n", asUser)
	}
	if finalReadonly {
		fmt.Println("Mode: readonly (all state-changing actions disabled)")
	}
	if finalAuthKeys != "" {
		fmt.Printf("Authentication: public keys from %s\n", finalAuthKeys)
	} else if finalPass != "" {
		fmt.Printf("Authentication: password protected via %s (loopback only)\n", passSource)
	} else {
		fmt.Println("Authentication: open (loopback only, any key accepted, readonly enforced)")
	}
	if openAuth {
		fmt.Println("WARNING: no --authorized-keys or --password: forcing readonly mode.")
		fmt.Println("WARNING: non-loopback binds require --authorized-keys; open-auth never listens beyond loopback.")
		fmt.Println("WARNING: loopback is not a user boundary: on a shared host every local account can connect and read journals and inspect output.")
	}
	if server.IsWildcardAddr(finalAddr) && !finalReadonly {
		fmt.Println("WARNING: listening on all interfaces with write access enabled; prefer -a 127.0.0.1:2222 or --readonly.")
	}
	if *pass != "" {
		fmt.Println("WARNING: --password is visible in the process list; prefer --password-file, QUADMAN_SERVE_PASSWORD, or --authorized-keys.")
	}
	fmt.Println("Press Ctrl+C to stop the server.")

	if err := server.Serve(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
	fmt.Println("\nquadman SSH daemon stopped gracefully.")
}

// servePasswordEnv carries the serve password without exposing it in the
// process list (unlike --password): process environments are owner-readable
// while argv is world-readable.
const servePasswordEnv = "QUADMAN_SERVE_PASSWORD" // #nosec G101 -- env var name, not a credential

// checkSecretFile reads a password file: it must exist, must not be a
// directory, and must be non-empty after trimming a trailing newline. A
// file readable beyond its owner yields a warning, not a refusal: managed
// secret mounts are commonly 0444 by design.
func checkSecretFile(path string) (content, warning string, err error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("password file %q: %w", path, err)
	}
	if st.IsDir() {
		return "", "", fmt.Errorf("password file %q is a directory", path)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- operator-provided secret path
	if err != nil {
		return "", "", fmt.Errorf("password file %q: %w", path, err)
	}
	content = strings.TrimRight(string(data), "\r\n")
	if content == "" {
		return "", "", fmt.Errorf("password file %q is empty", path)
	}
	if st.Mode().Perm()&0o077 != 0 {
		warning = fmt.Sprintf("WARNING: password file %s is readable beyond owner (mode %04o); prefer chmod 600", path, st.Mode().Perm())
	}
	return content, warning, nil
}

// resolveServePassword picks the serve password from exactly one source:
// --password, --password-file, or the environment win over config.yaml,
// which itself accepts either password or password_file. getenv is os.Getenv
// in production and a stub in tests. Errors never echo the secret.
func resolveServePassword(passFlag, passFileFlag string, scfg config.ServeSettings, getenv func(string) string) (password, source, warning string, err error) {
	set := 0
	if passFlag != "" {
		set++
	}
	if passFileFlag != "" {
		set++
	}
	env := ""
	if getenv != nil {
		env = getenv(servePasswordEnv)
	}
	if env != "" {
		set++
	}
	if set > 1 {
		return "", "", "", fmt.Errorf("specify only one of --password, --password-file, %s", servePasswordEnv)
	}
	switch {
	case passFlag != "":
		return passFlag, "--password", "", nil
	case passFileFlag != "":
		content, warn, err := checkSecretFile(passFileFlag)
		if err != nil {
			return "", "", "", err
		}
		return content, "--password-file", warn, nil
	case env != "":
		return env, servePasswordEnv, "", nil
	}
	if scfg.Password != "" && scfg.PasswordFile != "" {
		return "", "", "", fmt.Errorf("config serve: set only one of password, password_file")
	}
	if scfg.PasswordFile != "" {
		content, warn, err := checkSecretFile(scfg.PasswordFile)
		if err != nil {
			return "", "", "", err
		}
		return content, "config password_file", warn, nil
	}
	if scfg.Password != "" {
		return scfg.Password, "config password", "", nil
	}
	return "", "", "", nil
}

// resolveServeAddr picks the listen address: explicit --address wins, then
// --port and config ports (both bound to loopback), then the loopback
// default. Only an explicit address can ever bind beyond localhost.
func resolveServeAddr(addrFlag, portFlag string, scfg config.ServeSettings) string {
	if addrFlag != "" {
		return addrFlag
	}
	if portFlag != "" {
		return net.JoinHostPort(server.DefaultHost, portFlag)
	}
	if scfg.Address != "" {
		return scfg.Address
	}
	if scfg.Port != "" {
		return net.JoinHostPort(server.DefaultHost, scfg.Port)
	}
	return server.DefaultAddress
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
	host := "<host>"
	if server.IsLoopbackAddr(addr) {
		host = "127.0.0.1"
	}
	if port == "22" {
		return host
	}
	return "-p " + port + " " + host
}

func printSkill(format string) {
	s := skill.Get(moduleVersion())
	if format == "json" {
		out, err := s.JSON()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error formatting skill json:", err)
			os.Exit(1)
		}
		fmt.Println(out)
		return
	}
	fmt.Print(s.Markdown())
}
