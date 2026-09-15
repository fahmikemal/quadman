# Riset & Katalog Fitur quadman

Kompilasi riset ekosistem (Podman/Quadlet, TUI pembanding, Charm stack) untuk
menyusun arah pengembangan quadman. Disusun 2026-09-12 dari sumber primer
(dokumentasi resmi, source code generator Podman, GitHub, Context7).

---

## 1. Posisi strategis & diferensiasi

| Tool | Pendekatan | Quadlet? | Butuh podman socket? |
|---|---|---|---|
| **quadman** (kita) | TUI, CLI-only (`systemctl`/`journalctl`/`loginctl`) | ✅ inti | ❌ tidak |
| podman-tui (org resmi `containers`) | TUI, REST API via Go bindings | ❌ **tidak sama sekali** | ✅ wajib |
| quadletman | Web UI (HTMX), PAM auth, "compartments" | ✅ | ❌ |
| podman desktop / Portainer / Dockge | GUI desktop / compose-only | ❌/parsial | — |

**Kesimpulan positioning:**
1. **Niche quadman tervalidasi dan belum diperebutkan**: podman-tui tidak
   menyentuh Quadlet sama sekali (issue tracker-nya tidak ada request quadlet);
   satu-satunya pesaing langsung (quadletman) adalah web UI, beta, 57 stars.
2. **"CLI-only, tanpa socket" adalah diferensiator yang layak dipertahankan** —
   podman-tui *wajib* `systemctl --user start podman.socket`, sesuatu yang
   banyak user rootless hindari (permukaan serangan + kompleksitas).
   Semua command `podman` yang kita butuhkan (stats, events, df, quadlet
   list, auto-update) bekerja tanpa socket karena Podman daemonless.
3. **Pendekatan "journal = canonical log store" tervalidasi**: quadlet memaksa
   `--log-driver=journald --rm`, sehingga `journalctl` memang satu-satunya
   sumber log yang benar. Rencana follow-mode logs sejalan dengan arsitektur
   upstream, bukan melawannya.

---

## 2. Temuan ekosistem Podman / Quadlet

### 2.1 `podman quadlet` subcommand suite (Podman 5.3+) — temuan terbesar

Ada family command resmi yang overlap dengan apa yang quadman bangun manual:

- `podman quadlet list` — output: name, **systemd unit name**, path, **status**
  (`Not loaded`, `loaded template`, `active/running`, `inactive/dead`,
  `failed/failed`, `activating/start`, `deactivating/stop`), application, pod;
  `--filter name=|pod=|status=`, `--format json`.
- `podman quadlet install [FILE|URL|.quadlets]` — `--application` (grup app),
  `--replace`, **`--reload-systemd` (default true)** = auto daemon-reload.
- `podman quadlet rm` — `--all/--force/--recursive`, sadar template `@`.
- `podman quadlet print` — tampil isi quadlet termasuk komentar.

**Rekomendasi:** jangan ganti discovery internal (kita lebih cepat & tanpa
dependency podman), tapi **pakai sebagai enrichment opsional** — deteksi
`podman` di PATH, gabungkan status dari `podman quadlet list --format json`,
fallback ke `systemctl show` murni seperti sekarang. Status "loaded template"
dan pengelompokan application/pod adalah data yang tidak bisa kita dapat dari
systemctl.

### 2.2 Kemampuan file Quadlet yang perlu di-surface UI

Dari podman-systemd.unit(5) — fitur yang paling operasional:

- **Health**: `HealthCmd=`, `HealthOnFailure=kill` (restart via systemd!),
  `HealthOnFailure=restart|none`, `HealthStartPeriod=`, `HealthLogDestination=`,
  `Notify=healthy` (READY menunggu healthcheck lulus).
- **Auto-update**: `AutoUpdate=registry|local` per unit; user timer
  `podman-auto-update.timer` (jalan harian, butuh linger untuk unattended);
  `podman auto-update --dry-run --format json` untuk preview;
  `--rollback` default on (deteksi gagal paling akurat kalau `Notify=sdnotify`).
- **Drop-in directories** (quadman belum support sama sekali):
  `foo.container.d/*.conf` merge alfabetis; generic `container.d/` untuk semua
  unit tipe itu; **cascading dashed prefix** `foo-.container.d/`,
  `foo-bar-.container.d/` (spesifik menang atas generik); template baca dua
  sumber (`foo@inst.container.d` + `foo@.container.d`). UI harus memandang
  unit = base file + N drop-in yang di-merge.
- **`.quadlets` multi-document**: beberapa quadlet dalam satu file, pemisah
  `---`, header `# FileName=<name>`.
- **Dependency graph**: `[Unit]` Wants/Requires/BindsTo/PartOf/Upholds/Conflicts/
  Before/After otomatis diterjemahkan kalau merujuk unit quadlet lain;
  dependency implisit dari `Image=foo.image|foo.build`, `Network=`, `Volume=`,
  `Mount=`, `Pod=`, `Artifact=`. → peluang tampilan **tree topology**.
- **`[Install]` WantedBy=/RequiredBy=/Alias=** — berarti **enable/disable +
  "start on boot" adalah aksi inti yang quadman belum punya**
  (`systemctl --user enable --now <unit>` bekerja untuk unit hasil generator).
- **`[Quadlet] DefaultDependencies=false`** — mematikan dependency
  network-online implisit.
- **[Service] pass-through**: `TimeoutStartSec=` (mitigasi pull lambat),
  `Restart=always`, `RestartSec=`.

### 2.3 Pain points user (2014–2026) → fitur jawaban

| Pain point | Sumber | Fitur jawaban quadman |
|---|---|---|
| Edit file → lupa `daemon-reload` | semua tutorial | auto-detect mtime file > mtime generator run → banner "reload needed?" ; atau watch + prompt |
| `systemctl stop` **menghapus** container (`--rm`), writable layer hilang | podman#28002, discuss#26709 | tampilkan hint saat stop: "container will be removed; state lives in volumes" |
| Pull image lambat → `activating (timed-out)` di 90s default | podman docs, discuss#19521 | deteksi sub-state `timed-out` → hint `TimeoutStartSec`/`Pull=` |
| Log container lama tidak terlihat di `podman logs` | komunitas | **follow-mode journal** (sudah di roadmap) — journal memang satu-satunya sumber |
| `podman system prune` menghapus container quadlet yang stop | toolbox#1005, discuss#26899 | warning sebelum prune kalau ada unit inactive |
| Rootless storage lambat (fuse-overlayfs / NFS home) | mailing list, fedora discussion | screen storage: `podman system df --verbose`, tampilkan storage driver |
| Linger & pasta/slirp4netns migration | Arch wiki, reddit | indikator linger ✅ sudah ada; pertajam dengan saran |
| Migrasi dari docker-compose | banyak | integrasi `podlet compose` (lihat 2.4) |

### 2.4 podlet (kini di org `containers`, v0.3.2, aktif)

Mendukung: `docker run`/`podman run` (termasuk dari file skrip & URL) →
quadlet; **compose → per-service .container + wiring dependency otomatis**
(`--create-pods/networks/volumes`); konversi dari objek hidup
(`container/pod inspect`, `kube play`, `network/volume create`, `image
build/pull`); `podman artifact pull` → `.artifact`. Flag install:
`--install`, `--service-name`, `--part-of`, `--socket-activation-proxy`,
`--dry-run`, `--podman-version`.

**Rekomendasi:** roadmap "generate via podlet" tetap valid dan semakin matang —
shell out ke binary `podlet` (deteksi PATH), jangan reimplement. UI flow:
paste `docker run` / pilih compose file → preview hasil → tulis ke config dir
→ auto `daemon-reload`.

### 2.5 CLI podman berguna untuk manager (semuanya tanpa socket)

- `podman stats --no-stream --format json` — CPU/mem per container.
- `podman events --format json --since 1h` — streaming event (podman-tui
  punya dialog events dari ini).
- `podman system df --format --verbose` — disk usage per images/containers/volumes.
- `podman healthcheck run <container>` — exit 0/1/125.
- `podman ps -a --format json`, `podman image prune --filter until=24h`.
- `podman kube generate <container|pod>` → YAML untuk `.kube`.
- ⚠️ `podman generate systemd` **deprecated** — jangan pernah dibangun di atasnya.
- `podman secret create/ls/rm` (rootless) — dipakai quadletman untuk secret
  store per-user.

---

## 3. Pola UX TUI terbaik (lazydocker, k9s, lazygit, btop, podman-tui)

Pola terurut berdasarkan nilai untuk quadman:

1. **Follow-mode logs** (lazydocker, k9s): tail + ring buffer. Preseden angka:
   k9s `tail: 100` fetch / `buffer: 1000` view; lazydocker `since: 60m`.
2. **Auto-refresh polling** (k9s `refreshRate` default 2s): re-enumerate unit,
   **cursor dipin berdasarkan nama unit** (bukan indeks) supaya refresh tidak
   melompatkan seleksi.
3. **Filter-first `/`** (k9s, lazygit, lazydocker): regex/fuzzy di semua list —
   keybinding dengan leverage tertinggi yang belum dipunyai quadman.
4. **Destructive-op tiers** (k9s): konfirmasi ketik untuk delete; tier "force"
   tanpa konfirmasi yang eksplisit; **readonly mode** global.
5. **Uppercase = eskalasi scope** (lazydocker/lazygit): huruf kecil = unit
   terpilih, huruf besar = scope penuh (quadman sudah setengah ada: `R`).
6. **Detail pane bertab `[`/`]`, dua zona enter/esc** (lazydocker): tab
   file/status/journal/source; `+`/`_` siklus normal/half/fullscreen untuk
   terminal kecil.
7. **`tea.ExecProcess` untuk $EDITOR / journalctl -f penuh**: hand-over
   terminal lalu resume rapi.
8. **Help overlay kontekstual** (k9s `?`): tampilkan mnemonic yang *valid saat
   ini*, bukan daftar statis.
9. **Config YAML + custom commands** (lazydocker — fitur paling dicintai):
   custom command per-konteks dengan Go template, mis.
   `systemctl --user status {{ .UnitName }}`.
10. **Mouse off-by-default, keyboard parity penuh** (k9s `enableMouse: false`;
    btop klik semua tombol berlabel). Kill-switch karena mouse capture merusak
    text selection.
11. **Colorblind-safe + redundansi simbol**: jangan encode state lewat warna
    saja (≈8% pria buta warna merah-hijau); pasangkan warna dengan
    glyph/teks ("failed", ●/■); degrade 24-bit→256→16.
12. **OSC52 clipboard + native fallback** (k9s) — copy nama unit/image/log
    lewat SSH/tmux.
13. **Bulk/marked operations** (k9s space-mark): tandai baris → aksi massal
    start/stop/enable — natural untuk set quadlet.
14. **Status health style: long/short/icon + `noIcons`** (k9s/btop) untuk
    terminal tanpa nerd font.
15. **Recent-actions log** (lazygit undo `z` analog): trust-builder; untuk
    sistemd: log aksi + waktu, dan konfirmasi toggle linger.

Anti-pola (dari podman-tui): **modal form dialog-heavy** untuk semua aksi —
komunitas memilih model immediate-action lazydocker. Kecuali create/edit yang
memang butuh form, aksi harus one-key.

---

## 4. Inventaris Charm stack v2 (bubbles v2.2.1, bubbletea v2.0.9, lipgloss v2.0.6)

### 4.1 Komponen bubbles → pemakaian quadman

| Komponen | Status quadman | Peluang |
|---|---|---|
| `list` | ❌ | **fuzzy filtering built-in** (`SetFilteringEnabled(true)`, custom `FilterFunc`) + paginator + spinner embed — kandidat menggantikan/augmentasi table |
| `table` | ✅ dipakai | tidak punya filter built-in (terverifikasi source) — filter harus di-layer sendiri atau pindah `list` |
| `viewport` | ✅ dipakai | **`AtBottom()` + `GotoBottom()`** = primitif tepat untuk follow-mode stick-to-tail; `MouseWheelEnabled` |
| `tree` (baru di v2) | ❌ | **dependency graph quadlet ↔ unit ↔ pod/volume/network** — fitur showcase |
| `textinput` | ❌ | command palette `:`, filter prompt, input argumen journalctl |
| `textarea` | ❌ | draft quadlet (walau $EDITOR via ExecProcess lebih baik) |
| `key` + `help` | manual | `help.Model` footer + `key.Binding` per-mode — help selalu jujur |
| `spinner` | ❌ | saat restart/daemon-reload berjalan |
| `progress` | ❌ | bar uptime/metric (v2: `progress.WithColors`) |
| `filepicker` | ❌ | open-file constrained ke `~/.config/containers/systemd` |
| `stopwatch`/`timer` | ❌ | indikator "failed for HH:MM:SS" |

### 4.2 Pola streaming follow-logs (rekomendasi dari dokumentasi)

Satu pembacaan in-flight yang di-re-issue di Update (idiomatik, tidak bisa
interleave):

```go
case logLineMsg:
    wasAtBottom := m.viewport.AtBottom()      // stick-to-tail hanya jika sudah di bawah
    m.logs = append(m.logs, msg.line)          // ring buffer capped (mis. 1000)
    m.viewport.SetContent(strings.Join(m.logs, "\n"))
    if wasAtBottom || m.following {
        m.viewport.GotoBottom()
    }
    return m, followNextLine(unit)             // read berikutnya
```

Alternatif setara: goroutine pemilik scanner + `p.Send(msg)` per baris.
Catatan: tidak ada debounce resize bawaan — DIY; coalescing stream sangat
chatty bisa dibatasi `tea.WithFPS`.

### 4.3 `tea.ExecProcess` untuk $EDITOR

```go
editor := os.Getenv("EDITOR"); if editor == "" { editor = "vi" }
return m, tea.ExecProcess(exec.Command(editor, u.Path), func(err error) tea.Msg {
    return editorFinishedMsg{err}  // → re-read file + tawarkan daemon-reload
})
```

### 4.4 v2 View fields & lain-lain

- `View()` mengembalikan `tea.View` dengan field deklaratif: `AltScreen`,
  `MouseMode` (CellMotion/AllMotion), `ReportFocus`, `WindowTitle`,
  `Cursor`. Opsi lama (`WithAltScreen`, dll) **sudah dihapus di v2**.
- Pesan bawaan baru: `ClipboardMsg`, `BackgroundColorMsg` (→ auto light/dark),
  `ColorProfileMsg`, `SuspendMsg`/`ResumeMsg` (Ctrl+Z), `tea.WithWindowSize`
  (bagus untuk test), `tea.WithColorProfile`.
- lipgloss v2: `colorprofile.Detect` + `lipgloss.Complete(profile)` untuk
  warna per-profile; `lipgloss.LightDark(...)` pengganti AdaptiveColor;
  `JoinHorizontal` posisi fraksional untuk split-pane responsif;
  `charmbracelet/x/ansi` (Truncate/Wrap/Width) **sudah ada di go.sum** —
  truncating deskripsi/log tanpa dependency baru.
- `teatest` untuk e2e (send keys → golden output); VHS untuk GIF dokumentasi.

---

## 5. Katalog fitur quadman (hasil sintesis)

### Tier 0 — quick wins (hari)
- [ ] Initial commit + tag v0.1 + `goreleaser` (fondasi rilis).
- [ ] **Auto-refresh polling** (default 2–3s, cursor dipin per nama unit) + spinner saat aksi berjalan.
- [ ] **`enable`/`disable` (+ `--now`)** unit — aksi inti yang hilang ([Install]/WantedBy).
- [ ] Help overlay kontekstual (`help.Model` + `key.Binding` per mode).
- [ ] Enrichment opsional via `podman quadlet list --format json` (fallback rapi kalau podman tidak ada).
- [ ] Banner "daemon-reload needed?" saat mtime file > mtime terakhir reload (simpan state reload terakhir).

### Tier 1 — diferensiator utama (minggu)
- [ ] **Follow-mode logs**: `journalctl -f -n 200`, ring buffer 1000, AtBottom/GotoBottom, toggle pause `f`, hint bar.
- [ ] **Filter `/`** — pilih: ganti table→`list` (fuzzy built-in) atau table + textinput re-filter.
- [ ] **Edit file via $EDITOR** (`tea.ExecProcess`) → setelah exit: tawarkan daemon-reload otomatis.
- [ ] **Layar auto-update**: status `AutoUpdate=` per unit; `podman auto-update --dry-run --format json` preview; status/toggle `podman-auto-update.timer` user; sadar rollback.
- [ ] Kolom health: `podman healthcheck run` / state health dari `podman ps --format json`; aksi `h` run healthcheck.

### Tier 2 — kedalaman operasional
- [ ] **Drop-in directories**: parse & merge (`foo.container.d/*.conf` + cascading `foo-.container.d/`), view "effective unit" (base+dropins, dropin ditandai), edit per drop-in.
- [ ] **Dukungan `.quadlets`** multi-document (parse `# FileName=`).
- [ ] **Tree topology dependency** (`bubbles/tree`): pod↔container, Image=/Network=/Volume= implicit deps, [Unit] deps.
- [ ] Detail pane bertab (`[`/`]`): source / status systemctl / journal / inspect podman.
- [ ] Screen storage & events: `podman system df --verbose`, streaming `podman events --format json`, warning sebelum prune saat ada unit inactive.
- [ ] Hint pintar: sub-state `timed-out` → saran `TimeoutStartSec`/`Pull=`; stop → hint "container removed, state di volume".
- [ ] Generate via **podlet** (paste docker-run / compose → preview → install → reload).

### Tier 3 — infrastruktur & skala
- [ ] SSH/remote mode (roadmap lama) — via `podman --remote` + systemctl ssh, atau `quadman --host user@host`.
- [ ] Config YAML (XDG) + **custom commands** Go-template + keybinding customization.
- [ ] Tema (YAML, colorblind-safe default, auto light/dark via BackgroundColorMsg, degrade 256/16).
- [ ] Mouse opt-in + wheel di table/viewport; OSC52 clipboard.
- [ ] Bulk mark (`space`) + aksi massal; readonly mode (`--readonly`).
- [ ] `teatest` e2e; VHS GIF di README; skrip benchmark refresh 500+ unit.

### Prinsip yang dijaga
- Tetap **CLI-only** (tanpa podman socket) — diferensiasi vs podman-tui.
- `journalctl` tetap sumber log kanonik (sesuai desain quadlet upstream).
- Aksi one-key immediate; form/dialog hanya untuk create/edit (anti-pola podman-tui).
- Uppercase = eskalasi scope, lowercase = unit terpilih.

---

## 6. Sumber utama

- podman-tui: <https://github.com/containers/podman-tui> (+ issues)
- quadletman: <https://github.com/mikkovihonen/quadletman>
- podlet: <https://github.com/containers/podlet> (v0.3.2)
- podman-systemd.unit(5): <https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html>
- podman-auto-update(1): <https://docs.podman.io/en/latest/markdown/podman-auto-update.1.html>
- Source generator quadlet Podman: `pkg/systemd/quadlet/{quadlet,unitdirs}.go`
- lazydocker: <https://github.com/jesseduffield/lazydocker> (+ docs/keybindings, Config.md)
- k9s: <https://github.com/derailed/k9s>, <https://k9scli.io/topics/commands/>, config docs
- lazygit: <https://github.com/jesseduffield/lazygit>
- btop: <https://github.com/aristocratos/btop>
- Tips bubbletea: <https://leg100.github.io/en/posts/building-bubbletea-programs/>, <https://charm.land/blog/commands-in-bubbletea/>
- bubbletea v2 upgrade guide (module cache v2.0.9), bubbles v2.2.1, lipgloss v2.0.6 (via Context7)
- Pain points: podman#28002, podman discussions #26709/#19521/#26899, toolbox#1005, Arch wiki Podman, komunitas reddit/fedora/mailing-list (URL lengkap di laporan riset)

---

## Refresh — September 2026 (v2)

Verifikasi ulang referensi per 15 Sep 2026.

- **Podman 6.x line aktif**: terbaru v6.1.1 (2 Sep 2026, security patch
  CVE-2026-17106). v6.1.0 (12 Agu 2026) menambah key Quadlet `ImageVolume=`
  (equivalent `--image-volume`; nilai `anonymous` default, `bind` deprecated→
  alias anonymous, `tmpfs` didukung), memperbaiki race yang bisa membuat unit
  hasil generator korup, dan membuat error generator tampil di STDERR (bukan
  hanya /dev/kmsg) — membuat `systemd-analyze verify` dan output `--dryrun`
  berguna untuk fitur diagnostik quadman. v5.8.6 menambal CVE-2026-19730
  (`podman quadlet install --replace` tidak mentruncate file lama).
- **podman-tui v2.0.0 (6 Sep 2026)**: support Podman 6, keyboard shortcuts
  baru untuk dialog, ARM builds — dan tetap **tanpa manajemen Quadlet**
  (tidak disebut di changelog mana pun). Niche quadman masih kosong.
- **podlet v0.3.2 (18 Mei 2026)**: mencakup opsi Quadlet Podman 5.3–5.8
  termasuk `.artifact` dan `.quadlets`; belum mendukung key Podman 6.x
  (`ImageVolume=`). Integrasi podlet tetap valid tapi quadman bisa
  meng-cover gap Podman 6 di sisi parse/read.
- **Charm stack sudah terbaru** (proxy.golang.org, 15 Sep 2026):
  bubbletea v2.0.9, bubbles v2.2.1, lipgloss v2.0.6 — sama dengan go.mod,
  tidak ada upgrade yang perlu.
- **`.artifact` tidak lagi ditandai eksperimental** di docs terbaru
  (sebelumnya experimental ~Podman 5.5).
- **Dokumen quadlet terkini** tidak menambah tipe unit baru; 8 tipe tetap.

Sources:
<https://github.com/containers/podman/releases>,
<https://github.com/containers/podman-tui/releases>,
<https://github.com/containers/podlet/releases>,
<https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html>,
<https://proxy.golang.org/charm.land/bubbletea/v2/@latest> (d. v2/lipgloss/bubbles)
