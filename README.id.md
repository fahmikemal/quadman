<div align="center">

# quadman

**Terminal UI manager untuk unit rootless Podman [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html).**

<p align="center">
  <a href="README.md"><b>English</b></a> | <a href="README.id.md"><b>Bahasa Indonesia</b></a>
</p>

[![CI](https://github.com/fahmikemal/quadman/actions/workflows/ci.yml/badge.svg)](https://github.com/fahmikemal/quadman/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/fahmikemal/quadman)](https://github.com/fahmikemal/quadman/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/fahmikemal/quadman.svg)](https://pkg.go.dev/github.com/fahmikemal/quadman)
[![Go Report Card](https://goreportcard.com/badge/github.com/fahmikemal/quadman)](https://goreportcard.com/report/github.com/fahmikemal/quadman)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

Quadlet adalah standar yang direkomendasikan untuk menjalankan container rootless sebagai systemd service — namun perkakas bawaannya tersebar di antara `systemctl`, `journalctl`, dan `loginctl`. quadman menyatukan seluruh siklus hidup tersebut dalam satu TUI terpadu:

- Mendeteksi setiap berkas sumber Quadlet yang dibaca oleh generator systemd (`*.container`, `*.pod`, `*.kube`, `*.volume`, `*.network`, `*.image`, `*.build`, `*.artifact`), di seluruh hierarki direktori pencarian pengguna (`$XDG_RUNTIME_DIR/containers/systemd` → `~/.config/containers/systemd` → `/etc/containers/systemd/users[/UID]` → `/usr/share/containers/systemd/users[/UID]`), dengan mekanisme shadowing (*first-match*) yang identik dengan generator resmi Podman.
- Memetakan setiap berkas sumber ke unit systemd yang dihasilkan oleh generator (`webapp.container` → `webapp.service`, `stack.pod` → `stack-pod.service`, `cache.volume` → `cache-volume.service`, dst.), menghargai override `ServiceName=` dan template unit (`web@.container` → `web@.service`).
- Menampilkan status live secara real-time (active/inactive/failed, `not-found` saat Anda lupa melakukan `daemon-reload`) beserta image yang dikonfigurasi langsung di tabel utama — di-refresh secara otomatis setiap beberapa detik dengan posisi kursor tetap terkunci pada baris pilihan Anda.
- Operasi `start` / `stop` (dengan konfirmasi aman — Quadlet menjalankan container dengan opsi `--rm`) / `restart` / `enable` / `disable` / `daemon-reload` cukup dengan satu tombol cepat, masing-masing disertai indikator animasi spinner saat operasi berjalan.
- Memberikan peringatan (*warning banner*) jika berkas quadlet mengalami modifikasi setelah `daemon-reload` terakhir, sehingga Anda tidak lagi bingung mengapa perubahan konfigurasi belum aktif.
- **Follow-mode journals** — streaming live `journalctl -f` per unit dengan fitur auto-scroll ke baris terbawah (*stick-to-tail*), jeda streaming (`f`), live grep filter (`F`), filter prioritas log (`p`), ekspor ke `.log`/`.jsonl` (`S`), dan salin log ke clipboard via OSC52 (`c`).
- **Command Palette** (`Ctrl+P`) — modal overlay berbasis pencarian fuzzy untuk menemukan dan mengeksekusi semua aksi siklus hidup unit, navigasi layar/view, diagnostik, dan perintah kustom (*custom commands*) dengan validasi status kontekstual secara langsung.
- **Native Wish SSH Daemon** (`quadman serve`) — menyajikan antarmuka TUI langsung melalui protokol SSH tanpa perlu menginstal quadman di mesin klien penghubung, lengkap dengan autentikasi public key (`authorized_keys`), autentikasi password, pembuatan host key otomatis, dan mode dashboard *read-only*.
- **Mode Ganda Rootless & Sistem** (`--system`) — identitas utama tetap memprioritaskan rootless secara bawaan, namun mendukung penuh manajemen unit tingkat sistem (*rootful*) (`/etc/containers/systemd`, `/run/containers/systemd`) serta deteksi otomatis hak akses root (`sudo quadman`).
- Pencarian fuzzy `/` untuk memfilter daftar unit secara instan saat Anda mengetik.
- Mengedit berkas quadlet langsung di editor teks `$EDITOR` bawaan Anda dari dalam TUI; otomatis memberikan peringatan untuk me-reload generator setelah berkas disimpan.
- **Layar auto-update** (`u`) — preview `podman auto-update --dry-run` dan status timer `podman-auto-update.timer`, dapat diaktifkan/dinonaktifkan langsung dengan `U`.
- Pemantauan kesehatan container (*healthcheck*): container yang tidak sehat (*unhealthy*) langsung ditandai di kolom STATE, dan tombol `h` mengeksekusi `podman healthcheck run` pada unit yang dipilih.
- **Validasi bawaan (*built-in validation*)** — generator Quadlet resmi dijalankan secara *dry-run* untuk memvalidasi setiap berkas saat refresh; unit yang bermasalah ditandai dengan ✗/⚠ dan layar `v` menjelaskan detail error secara gamblang, termasuk deteksi versi Podman (*version-gating hints*).
- **Direktori Drop-in** — mendukung berkas konfigurasi tambahan `foo.container.d/*.conf` (termasuk *cascading* `foo-.container.d/` dan drop-in generik `container.d/`) yang ditampilkan menyatu (*merged*) sesuai urutan evaluasi generator systemd.
- **Pohon dependensi (*dependency tree*)** (`t`) — visualisasi relasi pods → containers → unit image/network/volume beserta dependensi direktif `[Unit]`, dirender dengan komponen pohon (*tree*).
- Dukungan mouse: klik baris untuk memilih, putar roda mouse untuk menggulir log; `y`/`Y` untuk menyalin nama unit/image ke clipboard via protokol OSC52 (berfungsi mulus bahkan melalui koneksi SSH).
- **Panel detail bertab** (`[`/`]`) — menampilkan kode sumber, `systemctl status`, live journal, dan `podman inspect` dari unit yang dipilih dalam satu tampilan terintegrasi.
- **Penyimpanan & Event** — tombol `g` untuk diagnosa disk `podman system df --verbose`, tombol `w` untuk live streaming `podman events` (dapat dijeda dengan `f`).
- **Petunjuk pintar (*smart hints*)** — jika start unit berstatus `timed-out`, TUI menyarankan penyesuaian `TimeoutStartSec=` atau `Pull=`; peringatan *crash-loop* dan pemakaian `AutoUpdate=` tanpa timer aktif juga langsung disorot pada layar diagnostik masalah.
- **Generator dari perintah apapun** (`n`) — salin-tempel perintah `docker run` atau jalur berkas docker-compose; utilitas [podlet](https://github.com/containers/podlet) akan mengonversinya, Anda dapat meninjau preview hasilnya, lalu tekan `y` untuk menulis berkas dan memicu reload generator secara otomatis.
- **Template & Bundel** — instansiasi template `web@.container` menjadi `web@prod.service` (`i`), serta preview dan instalasi bundel multi-dokumen `.quadlets` (`I`).
- Pengayaan data opsional melalui `podman quadlet list` (pengelompokan aplikasi/pod) jika binary podman terpasang di host; tetap berfungsi penuh 100% tanpa ketergantungan podman socket.
- **Indikator & Pengontrol Linger Pengguna** — `loginctl enable-linger` adalah satu konfigurasi krusial yang wajib dimiliki server container rootless (tanpa linger, container Anda akan mati saat user logout). quadman menampilkan status linger di bar navigasi bawah dan memungkinkan toggle status dengan satu tombol `L`.

quadman dirancang secara sengaja bersifat *engine-agnostic*: hanya memanggil CLI `systemctl --user` / `journalctl --user` / `loginctl`, sehingga kompatibel dengan semua instalasi Podman rootless dan sama sekali tidak membutuhkan daemon atau socket API Podman yang berjalan di latar belakang.

## Tangkapan Layar (Screenshots)

Tampilan tabel utama — seluruh sumber Quadlet, unit systemd terkait, status aktif, dan image kontainer, lengkap dengan banner peringatan jika berkas di disk berubah:

![quadman unit list](docs/screenshot-list.svg)

Memilih unit akan langsung melakukan streaming journal secara real-time (`journalctl -f`) tanpa perlu keluar dari antarmuka TUI — jeda dengan `f`, cari dengan `/`:

![quadman journal view](docs/screenshot-logs.svg)

## Instalasi

Unduh binary siap pakai (Linux amd64/arm64) dari halaman [Releases](https://github.com/fahmikemal/quadman/releases):

```sh
curl -LO https://github.com/fahmikemal/quadman/releases/latest/download/quadman_0.4.4_linux_amd64.tar.gz
tar -xzf quadman_0.4.4_linux_amd64.tar.gz && sudo install quadman /usr/local/bin/
```

Atau menggunakan Go:

```sh
go install github.com/fahmikemal/quadman@latest
```

Atau kompilasi langsung dari repositori:

```sh
make build   # ./quadman
```

Kebutuhan sistem: Linux dengan systemd, sesi pengguna (*user session* untuk mode rootless), dan `journalctl` untuk penampil log. Tanpa daemon latar belakang; mendukung satu berkas konfigurasi YAML opsional (lihat bagian Konfigurasi di bawah).

## Penggunaan

```sh
quadman                          # Buka antarmuka TUI (sesi user rootless secara default)
quadman --system                 # Mode tingkat sistem / rootful (/etc/containers/systemd)
sudo quadman                     # Deteksi otomatis root dan mengaktifkan mode sistem
quadman serve                    # Jalankan TUI sebagai SSH server via Wish daemon (port :2222)
quadman serve -p 2222 --readonly # Dashboard monitoring SSH mode baca-saja
quadman --readonly               # TUI dengan seluruh aksi pengubah status dinonaktifkan
quadman --ssh user@host          # Kelola server remote rootless melalui koneksi SSH
quadman --theme colorblind       # Pilihan tema: auto, dark, light, atau colorblind
quadman --mouse                  # Aktifkan navigasi klik mouse
quadman --quadlet-dir ~/quadlets # Direktori sumber Quadlet tambahan (dapat diulang)
quadman list                     # Output daftar non-interaktif untuk integrasi script/piping
quadman list --system            # Tampilkan daftar quadlet tingkat sistem
quadman --skill                  # Ekspor spesifikasi Agent Skill untuk AI (markdown)
quadman --skill --skill-format=json # Ekspor spesifikasi Agent Skill dalam format JSON
quadman -version
```

### Tombol Navigasi (Keybindings)

| Tombol | Aksi |
| ----- | --------------------------------------------- |
| `↑/↓` `j/k` | Berpindah navigasi pada daftar baris |
| `enter` | Melihat isi berkas sumber Quadlet |
| `Ctrl+P` | Command Palette (pencarian fuzzy dan eksekusi aksi unit atau perintah kustom) |
| `/` | Filter fuzzy daftar unit (ketik untuk menyaring, `esc` untuk menghapus filter) |
| `l` | Live journal tail (`f` jeda, `/` cari, `F` grep filter, `p` prioritas, `S` ekspor, `c` salin) |
| `s` / `x` / `r` | Start / stop (dengan konfirmasi) / restart unit — atau seluruh unit yang ditandai dengan `space` |
| `space` | Tandai/lepas tanda baris untuk aksi massal (*bulk actions*, `esc` untuk membersihkan tanda) |
| `e` | Aktifkan saat boot (*enable*, menyisipkan blok `[Install]` ke berkas dengan konfirmasi) |
| `d` | Nonaktifkan saat boot (*disable*, menghapus blok `[Install]` dengan konfirmasi) |
| `E` | Mengedit berkas Quadlet menggunakan `$EDITOR` |
| `u` | Layar auto-update (`U` untuk toggle status timer otomatis) |
| `h` | Menjalankan `podman healthcheck` pada unit yang dipilih |
| `v` | Layar diagnosa masalah — validasi generator terhadap seluruh berkas Quadlet |
| `t` | Pohon dependensi (*dependency tree*: pods, images, networks, volumes, dependensi `[Unit]`) |
| `i` | Instansiasi template unit (`web@.container` → `web@prod.service`) |
| `D` | Hapus unit (menghentikan kontainer dan menghapus berkas sumber dengan konfirmasi) |
| `y` / `Y` | Salin nama unit / image ke clipboard (OSC52, bekerja sempurna via SSH) |
| `[` / `]` | Berpindah tab detail: source / status / journal / inspect |
| `I` | Pasang bundel `.quadlets` (`podman quadlet install`) |
| `g` | Layar penyimpanan (*storage*: `podman system df --verbose`, `r` untuk refresh) |
| `T` | Layar timer systemd (melihat jadwal aktif, pemicu service, dan hitung mundur kalender) |
| `K` | Layar secret Podman (melihat penyimpanan secret Podman, driver, dan metadata) |
| `w` | Live streaming event podman (`f` untuk jeda) |
| `n` | Generator quadlet via podlet (`podman run ...`, `docker run ...`, singkatan `run ...`, atau `compose <path>`) |
| `A` | Log aksi terakhir (melihat perintah apa yang dijalankan, waktu, dan hasilnya) |
| `R` | `systemctl --user daemon-reload` (regenerasi unit setelah berkas Quadlet diedit) |
| `L` | Toggle linger pengguna (`loginctl enable-linger`) |
| `?` | Buka bantuan tombol (*help overlay*) |
| `q` / `esc` | Keluar / kembali ke layar sebelumnya |

## Konfigurasi

Pengaturan YAML opsional terletak di `~/.config/quadman/config.yaml` (seluruh opsi bersifat opsional; berkas yang korup/salah sintaks akan dilaporkan di bar status bawah dan tidak diabaikan secara diam-diam):

```yaml
refresh_interval: 5s   # Interval polling daftar (default 2.5s)
log_tail: 200          # Jumlah baris awal snapshot journal saat membuka log
log_buffer: 1000       # Batas maksimum baris buffer penampil log
readonly: false        # Sama dengan flag quadman --readonly
theme: auto            # auto, dark, light, atau colorblind
mouse: false           # Sama dengan flag quadman --mouse (off menjaga seleksi teks terminal tetap aktif)
quadlet_dirs:          # Direktori sumber Quadlet tambahan (sama dengan --quadlet-dir)
  - ~/quadlets

custom_commands:
  - name: status
    key: S
    run: systemctl --user status {{.UnitName}}
  - name: image
    key: P
    run: podman image inspect {{.Image}}
```

Perintah kustom (*custom commands*) dijalankan secara aman tanpa perantara shell: string `run` diekspansi sebagai template Go (`{{.Name}}`, `{{.UnitName}}`, `{{.Kind}}`, `{{.Image}}`), dipecah dengan tokenizer pemisah tanda kutip, dan dieksekusi langsung. Hasil eksekusi tampil di bar status bawah dan tercatat di log aksi terakhir (`A`). Preferensi editor teks saat pertama kali dipilih disimpan pada berkas `config.json` di direktori yang sama.

## Mode SSH Remote

```sh
quadman --ssh user@host
```

Setiap pemanggilan CLI (`systemctl`, `journalctl`, `loginctl`, `podman`) diteruskan melalui binary `ssh` lokal Anda — SSH key, ssh-agent, `known_hosts`, dan konfigurasi `~/.ssh/config` langsung bekerja otomatis tanpa perlu setup tambahan. Jika `ssh user@host true` berhasil, quadman dipastikan langsung bekerja.

Mode remote mempertahankan kendali penuh untuk operasi baca dan siklus hidup: daftar unit, status live, start / stop / restart, journal logs, storage, events, auto-update, healthcheck, linger, dan pohon dependensi (berkas dibaca via `cat` sesuai kebutuhan, tanpa sinkronisasi disk). Aksi yang memodifikasi berkas di host (seperti edit, enable/disable saat boot, instansiasi template, generate berkas, install bundel, dan delete) dibatasi dengan pesan penjelasan — kelola berkas secara langsung dengan menjalankan quadman di server target.

## SSH Server (Wish Daemon)

```sh
quadman serve                                          # Berjalan di port :2222 secara default
quadman serve -p 2222 --readonly                       # Dashboard pemantauan read-only via SSH
quadman serve --authorized-keys ~/.ssh/authorized_keys # Batasi akses hanya untuk kunci terdaftar
quadman serve --password rahasia123                    # Proteksi dengan autentikasi kata sandi
```

quadman menyertakan server SSH bawaan yang ditenagai oleh pustaka [Charm Wish](https://github.com/charmbracelet/wish) (`wish/v2`). Ketika dijalankan di server, siapapun di jaringan atau tim Anda dapat mengakses antarmuka TUI quadman dengan satu perintah terminal sederhana tanpa perlu memasang quadman di komputer mereka:

```sh
ssh -p 2222 user@host
```

Fitur utama SSH Server:
- **Nol dependensi klien**: Komputer yang menghubung hanya memerlukan terminal client standar `ssh`.
- **Host Key Otomatis**: Membuat kunci host ED25519 otomatis di `~/.config/quadman/host_ed25519` jika belum ditentukan.
- **Opsi Autentikasi**: Mendukung akses terbuka (default), verifikasi berkas `authorized_keys`, atau perlindungan kata sandi.
- **Mode Dashboard Readonly**: Tambahkan flag `--readonly` untuk membagikan akses pemantauan kepada rekan tim dengan aman tanpa risiko salah mematikan atau mengubah unit produksi.
- **Eksekusi Aman**: Pembukaan editor lokal `$EDITOR` dinonaktifkan secara aman pada sesi server SSH.

## Lokasi Berkas Quadlet

quadman memindai hierarki pencarian rootless generator resmi:
(`$XDG_RUNTIME_DIR/containers/systemd` → `~/.config/containers/systemd` → `/etc/containers/systemd/users[/UID]` → `/usr/share/containers/systemd/users[/UID]`),
ditambah direktori kustom Anda dari opsi `quadlet_dirs` di config.yaml atau flag berulang `--quadlet-dir` (misal repositori git yang berisi manifest infrastruktur Anda). Direktori tambahan ditempatkan setelah direktori standar, sehingga berkas bernama sama di direktori standar tetap mengutamakan *shadowing* — semantik generator tetap terjaga.

Unit yang hanya ada di direktori tambahan ditandai dengan simbol `~`: generator belum dapat memuatnya, sehingga upaya menyalakan unit tersebut akan ditolak dengan penjelasan hingga berkas tersebut disalin atau di-symlink ke direktori pencarian standar dan di-reload (`R`). Aturan ini berlaku konsisten baik pada antarmuka TUI maupun perintah non-interaktif `quadman list`.

## Kompatibilitas

quadman dirancang khusus untuk ekosistem **Podman 6.x** (terbaru: 6.1.1, Sep 2026) dan kompatibel penuh mulai dari Podman 5.3+ — rilis yang pertama kali memperkenalkan sub-perintah `podman quadlet list`. Seluruh komponen pustaka terminal menggunakan versi mutakhir: bubbletea v2.0.9, bubbles v2.2.1, dan lipgloss v2.0.6.

## Roadmap Pengembangan

quadman dirilis secara terstruktur dalam kelompok fitur (lihat [CONTRIBUTING.id.md](CONTRIBUTING.id.md)); kenaikan versi minor untuk tingkatan fitur baru (*feature tiers*), dan patch untuk akumulasi perbaikan bug.

### v0.2.0 — Tier 1: Diferensiator Utama (✅ Telah Dirilis)

- [x] Follow mode untuk journal (`journalctl -f` streaming, auto-scroll ke bawah)
- [x] Filter fuzzy `/` pada daftar unit
- [x] Edit berkas Quadlet via `$EDITOR` (`tea.ExecProcess`) → penawaran otomatis `daemon-reload` setelah keluar
- [x] Layar auto-update: status `AutoUpdate=` per unit, preview `podman auto-update --dry-run`, status/toggle timer `podman-auto-update.timer` pengguna
- [x] Kolom status kesehatan (Health) + tombol `h` mengeksekusi `podman healthcheck run`
- [x] Konfirmasi stop — Quadlet menjalankan kontainer dengan opsi `--rm`, aksi stop otomatis menghapus kontainer sementara
- [x] Pemetaan dan pembacaan direktif baru Podman 6.1 (contoh: `ImageVolume=`)
- [x] Pengujian otomatis untuk perintah non-interaktif `quadman list`

### v0.3.x — Tier 2: Kedalaman Operasional (✅ Telah Dirilis)

- [x] Validasi Quadlet bawaan (dry-run generator, penanda ✗/⚠, layar masalah, version gating)
- [x] Pengelolaan direktori drop-in (`*.container.d/*.conf`, cascading `foo-.container.d/`)
- [x] Tampilan pohon dependensi (bubbles `tree`): pod↔container, dependensi implisit `Image=`/`Network=`/`Volume=`, dependensi `[Unit]`
- [x] Navigasi mouse (klik untuk memilih baris, wheel scroll) + OSC52 clipboard (`y`/`Y`)
- [x] Instansiasi template (`web@.container` → `web@prod.service`)
- [x] Penghapusan unit terintegrasi (`podman quadlet rm --force` dengan mekanisme fallback)
- [x] Berkas bundel multi-dokumen `.quadlets` (header `# FileName=`) — deteksi, pratinjau, instalasi via `podman quadlet install` (`I`)
- [x] Panel detail bertab (`[`/`]`): source / status / journal / `podman inspect`
- [x] Layar penyimpanan dan event (`g` = `podman system df --verbose`, `w` = streaming `podman events`)
- [x] Petunjuk pintar (*smart hints*): deteksi `timed-out` → saran `TimeoutStartSec=`/`Pull=`, crash-loop start-limit, `AutoUpdate=` tanpa timer aktif
- [x] Generator berkas Quadlet via podlet (`n`): konversi `docker run` / compose → preview → tulis `y` → reload otomatis

### v0.4.0+ — Tier 3: Infrastruktur & Skala (✅ Telah Dirilis)

- [x] Mode SSH untuk server remote rootless (`--ssh user@host`; monitoring, siklus hidup, log, dan diagnostik via SSH — modifikasi berkas tetap terlindungi)
- [x] Konfigurasi YAML (kecepatan refresh, log tail/buffer, tema, mouse) + perintah kustom template Go
- [x] Tema ramah buta warna (*colorblind-safe*) + deteksi otomatis light/dark + opsi mouse
- [x] Penandaan massal (*bulk mark*) + aksi kelompok (`space` untuk menandai, `s`/`x`/`r` untuk mengeksekusi seluruh unit yang ditandai)
- [x] Mode baca-saja (`--readonly`)
- [x] Log riwayat aksi terkini (`A`: aksi apa yang dijalankan, kapan, dan status keberhasilannya)
- [x] Pengujian otomatis Update/View headless + benchmark 500 unit + demo tape VHS

### v0.4.2 — Tier 4: Enterprise, Multi-Mode & Akses Remote (✅ Telah Dirilis)

- [x] **Wish v2 SSH Daemon (`quadman serve`)** — server SSH terintegrasi ditenagai Charm Wish v2, koneksi langsung `ssh -p 2222 host`, auto-generate host key, autentikasi public key (`authorized_keys`), autentikasi kata sandi, mode read-only, dan manajemen sesi timeout.
- [x] **Command Palette (`Ctrl+P`)** — modal overlay pencarian fuzzy instan untuk eksekusi seluruh perintah siklus hidup unit, perkakas diagnostik, perpindahan layar, dan perintah pengguna dengan validasi status kontekstual secara langsung.
- [x] **Ekspor Log & Filter Interaktif** — ekspor log journal ke berkas terstempel waktu `.log` (teks biasa) dan `.jsonl` (format journal terstruktur) (`S`), salin ke clipboard OSC52 (`c`), live grep filter (`F`), dan toggle prioritas log (`p`: err, warning, info, all).
- [x] **Mode Native Sistem / Rootful (`--system`)** — dukungan kelas satu untuk unit Quadlet tingkat sistem (`/etc/containers/systemd`, `/run/containers/systemd`, `/usr/share/containers/systemd`), deteksi otomatis hak akses root (UID 0 / `sudo quadman`), penyesuaian dinamis argumen CLI `--user`, serta proteksi status linger sistem.

### v0.5.0 — Tier 5: Secrets, Timers & Agent Tooling (✅ Telah Dirilis)

- [x] **Integrasi Rahasia Podman (Secrets)** — inspeksi penyimpanan rahasia Podman (`K`), dan validasi referensi *pre-flight* otomatis untuk direktif `Secret=` pada berkas `.container` dengan peringatan dini di layar masalah (`v`).
- [x] **Tampilan Entitas Timer Systemd (`T`)** — inspeksi seluruh timer kalender yang aktif dan terjadwal (`systemctl list-timers`), pemicu eksekusi, serta hitung mundur waktu langsung di dalam TUI.
- [x] **Ekspor Agent Skill AI (`quadman --skill`)** — ekspor skema perkakas JSON yang dapat dibaca mesin dan dokumentasi Markdown komprehensif untuk LLM coding agents.

### Catatan Ekosistem (Sep 2026)

- [podman-tui](https://github.com/containers/podman-tui) v2.0.0 (6 September 2026) mendukung Podman 6 — dan **tetap tidak memiliki manajemen Quadlet**, membuktikan ceruk quadman tetap esensial.
- podlet v0.3.2 (Mei 2026) mencakup Podman ≤ 5.8; direktif baru Podman 6 seperti `ImageVolume=` belum dihasilkan oleh podlet.
- Unit `.artifact` tidak lagi berstatus eksperimental pada dokumentasi resmi terbaru; quadman memetakan namanya dengan tepat (`foo.artifact` → `foo-artifact.service`).

## Proyek Terkait

- [podman-tui](https://github.com/containers/podman-tui) — TUI resmi untuk Podman runtime (containers, pods, images). quadman melengkapinya di lapisan pengelolaan systemd/Quadlet.
- [podlet](https://github.com/k9withabone/podlet) — Generator CLI untuk berkas Quadlet.
- [quadletman](https://github.com/mikkovihonen/quadletman) — Web UI untuk manajemen Quadlet.

Tidak terafiliasi secara resmi dengan proyek Podman upstream.

## Lisensi

[MIT](LICENSE)
