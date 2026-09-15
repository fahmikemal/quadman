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
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
	"github.com/kemal-labs/quadman/internal/ui"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("quadman", moduleVersion())
		return
	}

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "list":
			list()
		case "version":
			fmt.Println("quadman", moduleVersion())
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q (available: list, version)\n", args[0])
			os.Exit(2)
		}
		return
	}

	if err := ui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// moduleVersion reports the release version. Builds injected via ldflags
// (make, goreleaser) take precedence; `go install module@version` builds
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
	units, err := quadlet.Discover()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(units) == 0 {
		fmt.Println("no quadlet units found in:")
		for _, d := range quadlet.SearchDirs() {
			fmt.Println("  " + d)
		}
		return
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

	statuses, serr := systemd.New().Show(context.Background(), unitNames(units))
	if serr != nil {
		fmt.Fprintln(os.Stderr, "warning:", serr)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "QUADLET\tKIND\tSYSTEMD UNIT\tSTATE\tSUB\tIMAGE")
	for i, u := range units {
		state, sub := "-", "-"
		if serr == nil {
			state, sub = statuses[u.UnitName].Display()
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", u.Name, u.Kind, u.UnitName, state, sub, images[i])
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func unitNames(units []quadlet.Unit) []string {
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.UnitName)
	}
	return names
}
