# Peta Jalan quadman

> Back to [README berbahasa Indonesia](README.id.md).

quadman dirilis secara terstruktur dalam kelompok fitur (lihat [CONTRIBUTING.id.md](CONTRIBUTING.id.md)); kenaikan versi minor untuk tingkatan fitur baru (*feature tiers*), dan patch untuk akumulasi perbaikan bug.

### Tier 1: Diferensiator Utama (✅ Telah Dirilis)

- [x] Follow mode untuk journal (`journalctl -f` streaming, auto-scroll ke bawah)
- [x] Filter fuzzy `/` pada daftar unit
- [x] Edit berkas Quadlet via `$EDITOR` (`tea.ExecProcess`) → penawaran otomatis `daemon-reload` setelah keluar
- [x] Layar auto-update: status `AutoUpdate=` per unit, preview `podman auto-update --dry-run`, status/toggle timer `podman-auto-update.timer` pengguna
- [x] Kolom status kesehatan (Health) + tombol `h` mengeksekusi `podman healthcheck run`
- [x] Konfirmasi stop — Quadlet menjalankan kontainer dengan opsi `--rm`, aksi stop otomatis menghapus kontainer sementara
- [x] Pemetaan dan pembacaan direktif baru Podman 6.1 (contoh: `ImageVolume=`)
- [x] Pengujian otomatis untuk perintah non-interaktif `quadman list`

### Tier 2: Kedalaman Operasional (✅ Telah Dirilis)

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

### Tier 3: Infrastruktur & Skala (✅ Telah Dirilis)

- [x] Mode SSH untuk server remote rootless (`--ssh user@host`; monitoring, siklus hidup, log, dan diagnostik via SSH — modifikasi berkas tetap terlindungi)
- [x] Konfigurasi YAML (kecepatan refresh, log tail/buffer, tema, mouse) + perintah kustom template Go
- [x] Tema ramah buta warna (*colorblind-safe*) + deteksi otomatis light/dark + opsi mouse
- [x] Penandaan massal (*bulk mark*) + aksi kelompok (`space` untuk menandai, `s`/`x`/`r` untuk mengeksekusi seluruh unit yang ditandai)
- [x] Mode baca-saja (`--readonly`)
- [x] Log riwayat aksi terkini (`A`: aksi apa yang dijalankan, kapan, dan status keberhasilannya)
- [x] Pengujian otomatis Update/View headless + benchmark 500 unit + demo tape VHS

### Tier 4: Enterprise, Multi-Mode & Akses Remote (✅ Telah Dirilis)

- [x] **Wish v2 SSH Daemon (`quadman serve`)** — server SSH terintegrasi ditenagai Charm Wish v2, koneksi langsung `ssh -p 2222 host`, auto-generate host key, autentikasi public key (`authorized_keys`), autentikasi kata sandi, mode read-only, dan manajemen sesi timeout.
- [x] **Command Palette (`Ctrl+P`)** — modal overlay pencarian fuzzy instan untuk eksekusi seluruh perintah siklus hidup unit, perkakas diagnostik, perpindahan layar, dan perintah pengguna dengan validasi status kontekstual secara langsung.
- [x] **Ekspor Log & Filter Interaktif** — ekspor log journal ke berkas terstempel waktu `.log` (teks biasa) dan `.jsonl` (format journal terstruktur) (`S`), salin ke clipboard OSC52 (`c`), live grep filter (`F`), dan toggle prioritas log (`p`: err, warning, info, all).
- [x] **Mode Native Sistem / Rootful (`--system`)** — dukungan kelas satu untuk unit Quadlet tingkat sistem (`/etc/containers/systemd`, `/run/containers/systemd`, `/usr/share/containers/systemd`), deteksi otomatis hak akses root (UID 0 / `sudo quadman`), penyesuaian dinamis argumen CLI `--user`, serta proteksi status linger sistem.

### Tier 5: Secrets, Timers & Agent Tooling (✅ Telah Dirilis)

- [x] **Integrasi Rahasia Podman (Secrets)** — inspeksi penyimpanan rahasia Podman (`K`), dan validasi referensi *pre-flight* otomatis untuk direktif `Secret=` pada berkas `.container` dengan peringatan dini di layar masalah (`v`).
- [x] **Tampilan Entitas Timer Systemd (`T`)** — inspeksi seluruh timer kalender yang aktif dan terjadwal (`systemctl list-timers`), pemicu eksekusi, serta hitung mundur waktu langsung di dalam TUI.
- [x] **Ekspor Agent Skill AI (`quadman --skill`)** — ekspor skema perkakas JSON yang dapat dibaca mesin dan dokumentasi Markdown komprehensif untuk LLM coding agents.

### Tier 6: Operasi, Hardening & Kompartemen (✅ Telah Dirilis)

- [x] **Exec shell (`X`)** — buka `/bin/sh` di kontainer unit (cocok nama persis, prefill `systemd-<name>`), ditolak via SSH/sesi serve dengan penjelasan.
- [x] **Statistik resource (`o`)** — tabel `podman stats --no-stream --all` (CPU/MEM/NET/BLOCK/PIDS), read-only dan aman via remote.
- [x] **System prune (`P`)** — `podman system prune -f` di balik konfirmasi yang menyebut unit Quadlet nonaktif yang berisiko.
- [x] **Generate dari objek hidup** — `n` menerima `container|pod|network|volume|image <name>` via `podlet generate`, plus aksi palette **Pull Unit Image** dengan fast-path bila image sudah ada.
- [x] **Kedalaman problems** — deteksi `[Kube] Yaml=` hilang (lokal + remote), hint timer `AutoUpdate=local` dan `.kube`, `Upholds=`/`Conflicts=` di dependency tree.
- [x] **Hardening serve** — akses terbuka (tanpa key/password) memaksa readonly + peringatan keras; perbandingan password constant-time; peringatan YAML password terbaca-publik; export log tak pernah menimpa (`-1`, `-2`…).
- [x] **Kompartemen (`--as <user>`, `quadman --as <user> list`)** — kelola Quadlet milik OS user lain via sudo non-interaktif dengan search path, systemd instance, storage, dan secret miliknya; switcher palette, chip title `[user]`, guard sesi terisolasi di semua jalur.
- [x] **Guard custom-key** — peringatan startup bila tombol `custom_commands` tertutup binding bawaan (bawaan selalu menang).

### Batch hardening serve (✅ Telah Dirilis)

- [x] **Kebijakan bind serve**: default loopback saja (`127.0.0.1:2222`); bind non-loopback menolak berjalan tanpa `--authorized-keys`, sehingga open-auth dan password-only hanya di loopback; open-auth tetap memaksa readonly di mana pun; berkas keys yang hilang atau kosong gagal keras saat startup; host key yang sudah ada dikencangkan ke `0600`.
- [x] **Guard CLI**: flag salah posisi setelah `list` ditolak; target `e2e` Makefile diperbaiki (path binary plus phony).
- [x] **Perbaikan skill**: entri ekspor mandiri, header versi satu-v, bentuk `--system list`, deskripsi kebijakan bind serve.
- [x] **Refresh Charm stack**: bubbletea v2.0.9 → v2.0.10; `govulncheck` dan `gosec` hijau.

### v0.1.0-dev1: password serve, serve --as & guard (✅ Telah Dirilis)

- [x] **Sumber password serve**: `--password-file`, `QUADMAN_SERVE_PASSWORD`, config `password_file` (tepat satu); secret tidak pernah muncul di argv.
- [x] **serve --as**: menyajikan sesi milik user lain via kompartemen sudo dengan validasi startup; guard sesi terisolasi tetap berlaku.
- [x] **Cakupan guard flag**: `version` dan `skill` menolak flag trailing (`skill` tetap mengizinkan `--format`/`--json` miliknya).
- [x] **Drop-in remote (read-only)**: dienumerasi via session runner untuk sesi SSH, kompartemen, dan serve.
- [x] **Guard mode**: `--system` dan `--as` ditolak bila digabung (CLI plus palette), karena unit sistem di luar sesi user mana pun.

### Catatan Ekosistem (Sep 2026)

- [podman-tui](https://github.com/containers/podman-tui) v2.0.0 (6 September 2026) mendukung Podman 6 — dan **tetap tidak memiliki manajemen Quadlet**, membuktikan ceruk quadman tetap esensial.
- podlet v0.3.2 (Mei 2026) mencakup Podman ≤ 5.8; direktif baru Podman 6 seperti `ImageVolume=` belum dihasilkan oleh podlet.
- Unit `.artifact` tidak lagi berstatus eksperimental pada dokumentasi resmi terbaru; quadman memetakan namanya dengan tepat (`foo.artifact` → `foo-artifact.service`).
