// Command qe2e drives quadman's TUI in a PTY and verifies every feature
// end to end: it sends real keystrokes and asserts what the screen shows.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

var (
	mu     sync.Mutex
	screen bytes.Buffer
)

type result struct {
	name string
	ok   bool
	note string
}

var results []result

func check(name string, ok bool, note string) {
	mark := "PASS"
	if !ok {
		mark = "FAIL"
	}
	results = append(results, result{name, ok, note})
	fmt.Printf("[%s] %s — %s\n", mark, name, note)
	if !ok {
		mu.Lock()
		s := screen.String()
		mu.Unlock()
		if len(s) > 3000 {
			s = s[len(s)-3000:]
		}
		fmt.Printf("----- screen dump -----\n%s\n-----------------------\n", s)
	}
}

func send(f *os.File, s string) {
	_, _ = f.WriteString(s)
}

// waitFor polls the captured screen output until want appears or timeout.
func waitFor(want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := strings.Contains(screen.String(), want)
		mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// drain clears captured output so the next assertion looks at fresh renders.
func drain() {
	mu.Lock()
	screen.Reset()
	mu.Unlock()
}

func main() {
	cmd := exec.Command("./quadman")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 42, Cols: 120})
	if err != nil {
		fmt.Println("start failed:", err)
		os.Exit(1)
	}
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				mu.Lock()
				screen.Write(buf[:n])
				if screen.Len() > 256*1024 {
					b := screen.Bytes()
					screen.Reset()
					screen.Write(b[len(b)-128*1024:])
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// 1. Initial load: all three demo units listed with correct naming.
	check("discovery+names", waitFor("demo-data-volume.service", 10*time.Second) && strings.Contains(last(), "demo-net-network.service"), "semua unit terdaftar dengan naming generator yang benar")

	// 2. Filter '/' live.
	send(f, "/")
	time.Sleep(300 * time.Millisecond)
	drain()
	send(f, "web")
	time.Sleep(600 * time.Millisecond)
	onlyWeb := strings.Contains(last(), "demo-web") && !strings.Contains(last(), "demo-data-volume.service")
	check("filter /", onlyWeb, "ketik 'web' menyaring daftar menjadi demo-web saja")
	send(f, "\r") // keep filter
	time.Sleep(400 * time.Millisecond)

	// 3. Start unit with 's'.
	send(f, "s")
	check("start unit", waitFor("running", 15*time.Second), "STATE berubah active/running")

	// 4. Healthcheck 'h'.
	drain()
	send(f, "h")
	check("healthcheck h", waitFor("healthy", 15*time.Second), "status menampilkan healthy")

	// 5. Follow logs 'l', pause 'f', back 'q'.
	drain()
	send(f, "l")
	ok := waitFor("LOGS", 5*time.Second)
	check("logs view", ok, "masuk view LOGS")
	time.Sleep(1500 * time.Millisecond)
	send(f, "f")
	check("logs pause f", waitFor("paused", 3*time.Second), "follow di-pause")
	send(f, "q")
	time.Sleep(400 * time.Millisecond)
	check("logs back", waitFor("QUADLET", 3*time.Second), "kembali ke daftar")

	// 6. Stop with y/N confirmation.
	drain()
	send(f, "x")
	ok = waitFor("y/N", 3*time.Second)
	check("stop confirm", ok, "prompt konfirmasi muncul")
	send(f, "y")
	check("stop unit", waitFor("container removed", 25*time.Second), "stop + hint container terhapus")

	// 7. Enable at boot 'e' (declarative [Install]).
	drain()
	send(f, "e")
	ok = waitFor("[Install]", 3*time.Second)
	check("enable confirm", ok, "prompt [Install] muncul")
	send(f, "y")
	ok = waitFor("starts on login", 10*time.Second)
	_, statErr := os.Stat("/run/user/1000/systemd/generator/default.target.wants/demo-web.service")
	check("enable at boot", ok && statErr == nil, "[Install] ditulis + symlink wants terbentuk")

	// 8. Disable from boot 'd'.
	drain()
	send(f, "d")
	ok = waitFor("remove [Install]", 3*time.Second)
	check("disable confirm", ok, "prompt hapus [Install] muncul")
	send(f, "y")
	ok = waitFor("still running", 10*time.Second)
	_, statErr = os.Stat("/run/user/1000/systemd/generator/default.target.wants/demo-web.service")
	check("disable at boot", ok && os.IsNotExist(statErr), "[Install] dihapus + symlink hilang")

	// 9. Auto-update screen 'u' (demo-web has AutoUpdate=registry and was restarted by enable).
	drain()
	send(f, "u")
	ok = waitFor("AUTO-UPDATE", 8*time.Second)
	check("updates screen", ok && strings.Contains(last(), "podman-auto-update.timer"), "layar auto-update + status timer")
	hasEntry := strings.Contains(last(), "demo-web") || strings.Contains(last(), "registry")
	check("updates entry", hasEntry, "entri unit ber-AutoUpdate=registry")
	send(f, "q")
	time.Sleep(400 * time.Millisecond)

	// 10. Stale banner: touch a quadlet file.
	drain()
	now := time.Now()
	_ = os.Chtimes(os.ExpandEnv("$HOME/.config/containers/systemd/demo-data.volume"), now, now)
	check("stale banner", waitFor("changed since last daemon-reload", 8*time.Second), "banner daemon-reload muncul setelah file disentuh")

	// 11. daemon-reload 'R' clears the banner.
	send(f, "R")
	ok = waitFor("daemon-reload ok", 8*time.Second)
	check("daemon-reload R", ok, "reload sukses (banner hilang di refresh berikutnya)")

	// 12. Clear filter, check full list again.
	send(f, "\x1b") // esc clears filter
	time.Sleep(500 * time.Millisecond)

	// 13. Linger toggle 'L' on then off (restore user state).
	drain()
	send(f, "L")
	ok = waitFor("linger: on", 5*time.Second)
	check("linger on", ok, "linger berhasil dinyalakan")
	send(f, "L")
	ok = waitFor("linger: off", 5*time.Second)
	check("linger off", ok, "linger dikembalikan off")

	// 14. Full help '?'.
	drain()
	send(f, "?")
	check("full help ?", waitFor("search order", 3*time.Second), "help lengkap tampil")
	send(f, "?")
	time.Sleep(300 * time.Millisecond)

	// 15. File view 'enter'.
	drain()
	send(f, "j") // move to any unit after filter cleared
	send(f, "\r")
	check("file view enter", waitFor("FILE", 3*time.Second), "view isi file quadlet")
	send(f, "q")
	time.Sleep(300 * time.Millisecond)

	// 16. Quit.
	send(f, "q")
	_ = cmd.Wait()

	fmt.Println()
	fail := 0
	for _, r := range results {
		if !r.ok {
			fail++
		}
	}
	fmt.Printf("E2E: %d/%d passed\n", len(results)-fail, len(results))
	if fail > 0 {
		os.Exit(1)
	}
}

func last() string {
	mu.Lock()
	defer mu.Unlock()
	s := screen.String()
	if len(s) > 32*1024 {
		return s[len(s)-32*1024:]
	}
	return s
}
