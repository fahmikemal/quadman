package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

// TestGenerateScreenshots renders the real Model into docs/screenshot-*.svg
// for the README. Regenerate with: QUADMAN_SCREENSHOTS=1 go test ./internal/ui -run Screenshots
func TestGenerateScreenshots(t *testing.T) {
	if os.Getenv("QUADMAN_SCREENSHOTS") == "" {
		t.Skip("set QUADMAN_SCREENSHOTS=1 to regenerate docs screenshots")
	}

	m := demoModel(t)

	listSVG := ansiToSVG(m.View().Content, "quadman - rootless quadlet manager")
	writeScreenshot(t, filepath.Join("..", "..", "docs", "screenshot-list.svg"), listSVG)

	logModel := demoModel(t)
	model, _ := logModel.Update(tea.WindowSizeMsg{Width: 124, Height: 16})
	logModel = model.(Model)
	model, _ = logModel.Update(logsMsg{unit: "webapp.service", out: sampleJournal})
	logModel = model.(Model)
	logsSVG := ansiToSVG(logModel.View().Content, "quadman - journal: webapp.service")
	writeScreenshot(t, filepath.Join("..", "..", "docs", "screenshot-logs.svg"), logsSVG)
}

func writeScreenshot(t *testing.T, path, svg string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func demoModel(t *testing.T) Model {
	t.Helper()
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 16})
	m = model.(Model)

	units := []quadlet.Unit{
		{Name: "api", Kind: quadlet.KindContainer, UnitName: "api.service"},
		{Name: "db", Kind: quadlet.KindContainer, UnitName: "db.service"},
		{Name: "stack", Kind: quadlet.KindPod, UnitName: "stack-pod.service"},
		{Name: "webapp", Kind: quadlet.KindContainer, UnitName: "webapp.service"},
		{Name: "webdata", Kind: quadlet.KindVolume, UnitName: "webdata-volume.service"},
	}
	images := []string{
		"ghcr.io/demo/api:v2.4",
		"docker.io/library/postgres:16",
		"",
		"docker.io/library/nginx:1.27",
		"",
	}
	statuses := map[string]systemd.Status{
		"api.service":            {Id: "api.service", LoadState: "loaded", ActiveState: "active", SubState: "running", Description: "Demo API"},
		"db.service":             {Id: "db.service", LoadState: "loaded", ActiveState: "failed", SubState: "failed", Description: "Demo database"},
		"stack-pod.service":      {Id: "stack-pod.service", LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
		"webapp.service":         {Id: "webapp.service", LoadState: "loaded", ActiveState: "active", SubState: "running", Description: "Web App"},
		"webdata-volume.service": {Id: "webdata-volume.service", LoadState: "loaded", ActiveState: "active", SubState: "exited"},
	}
	model, _ = m.Update(refreshMsg{
		units:    units,
		images:   images,
		statuses: statuses,
		linger:   true,
		lingerOK: true,
		stale:    []quadlet.Unit{{Name: "metrics", Kind: quadlet.KindContainer, UnitName: "metrics.service"}},
	})
	return model.(Model)
}

const sampleJournal = `2026-09-12T09:41:01+07:00 demo-host webapp[8121]: 2026/09/12 09:41:01 [notice] 1#1: using the "epoll" event method
2026-09-12T09:41:01+07:00 demo-host webapp[8121]: 2026/09/12 09:41:01 [notice] 1#1: start worker processes
2026-09-12T09:41:01+07:00 demo-host webapp[8121]: 2026/09/12 09:41:01 [notice] 1#1: start worker process 8122
2026-09-12T09:41:01+07:00 demo-host webapp[8121]: 2026/09/12 09:41:01 [notice] 1#1: start worker process 8123
2026-09-12T09:43:37+07:00 demo-host webapp[8121]: 172.19.0.4 - - [12/Sep/2026:09:43:37 +0700] "GET /healthz HTTP/1.1" 200 0 "-" "kube-probe/1.31"
2026-09-12T09:43:42+07:00 demo-host webapp[8121]: 172.19.0.4 - - [12/Sep/2026:09:43:42 +0700] "GET /healthz HTTP/1.1" 200 0 "-" "kube-probe/1.31"
2026-09-12T09:44:19+07:00 demo-host webapp[8121]: 172.19.0.9 - - [12/Sep/2026:09:44:19 +0700] "GET / HTTP/1.1" 200 615 "-" "Mozilla/5.0"
2026-09-12T09:44:20+07:00 demo-host webapp[8121]: 172.19.0.9 - - [12/Sep/2026:09:44:20 +0700] "GET /static/app.css HTTP/1.1" 200 4210 "https://demo.local/" "Mozilla/5.0"`

// --- ANSI SGR → SVG -------------------------------------------------------

type svgRun struct {
	text string
	fg   string
	bg   string
	bold bool
}

// parseANSILine walks a rendered line and splits it into styled runs.
func parseANSILine(s string) []svgRun {
	var runs []svgRun
	var fg, bg string
	bold := false
	cur := svgRun{}
	flush := func() {
		if cur.text != "" {
			runs = append(runs, cur)
		}
		cur = svgRun{}
	}
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "\x1b[") {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				break
			}
			params := s[i+2 : i+end]
			i += end + 1
			flush()
			fg, bg, bold = applySGR(params, fg, bg, bold)
			cur.fg, cur.bg, cur.bold = fg, bg, bold
			continue
		}
		// Append one full rune (not one byte): string(byte) would
		// reinterpret bytes >= 0x80 as latin-1 and mangle multibyte
		// characters such as —, → and · in the SVG output.
		_, size := utf8.DecodeRuneInString(s[i:])
		cur.text += s[i : i+size]
		i += size
	}
	flush()
	return runs
}

// applySGR mutates fg/bg/bold from one SGR sequence's parameters.
func applySGR(params, fg, bg string, bold bool) (string, string, bool) {
	if params == "" || params == "0" {
		return "", "", false
	}
	ps := strings.Split(params, ";")
	for j := 0; j < len(ps); j++ {
		n, err := strconv.Atoi(ps[j])
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			fg, bg, bold = "", "", false
		case n == 1:
			bold = true
		case n == 22:
			bold = false
		case n >= 30 && n <= 37:
			fg = xterm256(n)
		case n >= 90 && n <= 97:
			fg = xtermBase16[n-90+8]
		case n == 39:
			fg = ""
		case n >= 40 && n <= 47:
			bg = xterm256(n)
		case n >= 100 && n <= 107:
			bg = xtermBase16[n-100+8]
		case n == 49:
			bg = ""
		case n == 38 && j+1 < len(ps):
			if ps[j+1] == "5" && j+2 < len(ps) {
				if v, err := strconv.Atoi(ps[j+2]); err == nil {
					fg = xterm256(v)
				}
				j += 2
			} else if ps[j+1] == "2" && j+4 < len(ps) {
				fg = "#" + ps[j+2] + ps[j+3] + ps[j+4]
				j += 4
			}
		case n == 48 && j+1 < len(ps):
			if ps[j+1] == "5" && j+2 < len(ps) {
				if v, err := strconv.Atoi(ps[j+2]); err == nil {
					bg = xterm256(v)
				}
				j += 2
			} else if ps[j+1] == "2" && j+4 < len(ps) {
				bg = "#" + ps[j+2] + ps[j+3] + ps[j+4]
				j += 4
			}
		}
	}
	return fg, bg, bold
}

var xtermBase16 = [16]string{
	"#000000", "#800000", "#008000", "#808000",
	"#000080", "#800080", "#008080", "#c0c0c0",
	"#808080", "#ff0000", "#00ff00", "#ffff00",
	"#0000ff", "#ff00ff", "#00ffff", "#ffffff",
}

// xterm256 maps an xterm palette index to #rrggbb (80-15 map onto the bright
// variants like most terminals).
func xterm256(n int) string {
	switch {
	case n < 16:
		return xtermBase16[n]
	case n >= 232:
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	n -= 16
	levels := []int{0, 95, 135, 175, 215, 255}
	return fmt.Sprintf("#%02x%02x%02x",
		levels[(n/36)%6], levels[(n/6)%6], levels[n%6])
}

const (
	svgCharW    = 9.6
	svgLineH    = 25.0
	svgFont     = `ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace`
	svgFontSize = 15
	svgPadX     = 22.0
	svgPadTop   = 46.0
)

// ansiToSVG wraps rendered TUI output in a terminal-window SVG.
func ansiToSVG(content, title string) string {
	lines := strings.Split(content, "\n")
	maxCols := 0
	for _, l := range lines {
		if c := ansiWidth(l); c > maxCols {
			maxCols = c
		}
	}
	width := svgPadX*2 + float64(maxCols)*svgCharW
	height := svgPadTop + float64(len(lines))*svgLineH + 18

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="%s" xml:space="preserve">`,
		width, height, width, height, svgFont)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%.0f" height="%.0f" rx="10" fill="#10121c" stroke="#2a2e42"/>`, width-1, height-1)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%.0f" height="34" rx="10" fill="#191c2b"/>`, width-1)
	fmt.Fprintf(&b, `<rect x="0.5" y="26" width="%.0f" height="9" fill="#191c2b"/>`, width-1)
	for i, c := range []string{"#ff5f56", "#ffbd2e", "#27c93f"} {
		fmt.Fprintf(&b, `<circle cx="%.0f" cy="18" r="6" fill="%s"/>`, 22+float64(i)*20, c)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="23" font-size="12.5" fill="#8b91af">%s</text>`, width/2-float64(len(title))*3.6, title)

	for li, line := range lines {
		col := 0
		y := svgPadTop + float64(li)*svgLineH
		for _, r := range parseANSILine(line) {
			n := ansiWidth(r.text)
			if n == 0 {
				continue
			}
			x := svgPadX + float64(col)*svgCharW
			if r.bg != "" {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`,
					x, y-svgLineH+5, float64(n)*svgCharW, svgLineH, r.bg)
			}
			weight := ""
			if r.bold {
				weight = ` font-weight="bold"`
			}
			fg := r.fg
			if fg == "" {
				fg = "#d8dcec"
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="%d" fill="%s"%s textLength="%.1f" lengthAdjust="spacingAndGlyphs">%s</text>`,
				x, y-4, svgFontSize, fg, weight, float64(n)*svgCharW, xmlEscape(r.text))
			col += n
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// ansiWidth counts visible cells in a rendered string, skipping SGR sequences.
// It counts runes, not bytes, so multibyte characters (—, →, ·) count as one cell.
func ansiWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "\x1b[") {
			if end := strings.IndexByte(s[i:], 'm'); end >= 0 {
				i += end + 1
				continue
			}
			break
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return n
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func TestANSIToSVGParsing(t *testing.T) {
	runs := parseANSILine("\x1b[1;97;48;5;62mhi\x1b[m there")
	if len(runs) != 2 {
		t.Fatalf("runs = %+v, want 2", runs)
	}
	if runs[0].text != "hi" || !runs[0].bold || runs[0].fg != "#ffffff" || runs[0].bg != "#5f5fd7" {
		t.Errorf("run0 = %+v", runs[0])
	}
	if runs[1].text != " there" || runs[1].bold || runs[1].fg != "" {
		t.Errorf("run1 = %+v", runs[1])
	}
	if xterm256(62) != "#5f5fd7" || xterm256(241) != "#626262" || xterm256(42) != "#00d787" {
		t.Errorf("palette mapping broken: 62=%s 241=%s 42=%s", xterm256(62), xterm256(241), xterm256(42))
	}
	// Multibyte runes must survive byte-for-byte (no latin-1 mojibake)
	// and count as a single cell each.
	multi := parseANSILine("a—b→c·l")
	var got strings.Builder
	for _, r := range multi {
		got.WriteString(r.text)
	}
	if got.String() != "a—b→c·l" {
		t.Errorf("multibyte round-trip = %q, want %q", got.String(), "a—b→c·l")
	}
	if w := ansiWidth("\x1b[1ma—b→c·l\x1b[m"); w != 7 {
		t.Errorf("ansiWidth(multibyte) = %d, want 7", w)
	}
}
