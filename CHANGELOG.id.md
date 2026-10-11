# Changelog

> English: [CHANGELOG.md](CHANGELOG.md).

Semua perubahan penting quadman dicatat di sini. Format mengikuti
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Diubah

Desain ulang TUI (DESIGN.md v0.2). Chrome dan daftar unit:

- Palet 13 token baru untuk tema dark, light, dan colorblind. Latar
  utama tidak pernah dicat; hanya top bar, status line, tiles, modal, dan
  baris terpilih yang diberi surface.
- Setiap status tampil sebagai glyph + kata (`● active`, `✗ failed`,
  `▲ unhealthy`, `◐ starting`, `◌ not-found`, `◇ template`), tidak pernah
  hanya warna. `--ascii` (otomatis di `TERM=linux`) memakai glyph ASCII.
- Top bar menampilkan `quadman › scope › target` beserta chip interval
  refresh dan versi podman. Nav strip memuat semua layar beserta key dan
  badge-nya (problems, update tertunda).
- Daftar unit kini punya dashboard: enam tiles (units, running, failed,
  generator, linger, auto-update) dan blok "needs attention" berisi
  maksimal 4 masalah beserta key untuk memperbaikinya. Di bawah 100
  kolom atau 30 baris, dashboard diringkas jadi satu baris; `z` mengubah
  tampilannya, dan `dashboard: false` di config.yaml membuatnya ringkas
  sejak awal.
- Kolom tabel kini STATE · QUADLET · KIND · SUB · UPTIME · IMAGE.
  Masalah diurutkan paling atas (failed → unhealthy → starting →
  not-found → active → inactive) dan `S` berganti ke urutan nama atau
  kind. Di bawah 100 kolom, UPTIME dan IMAGE disembunyikan. Nama unit
  systemd pindah ke layar detail.
- Mulai 160 kolom, panel ringkasan unit terpilih tampil di samping
  daftar.
- Footer dikelompokkan (UNIT · INSPECT · VIEWS · HOST, plus SELECT mulai
  160 kolom), maksimal 2 baris, dan selalu diakhiri
  `^P all commands  ? help`. Chip linger, readonly, dan system pindah ke
  status line bersama posisi kursor.
- Command palette mengambang di atas layar asalnya: kotak 84 kolom di
  tengah (lembar selebar layar di bawah 100 kolom), aksi dikelompokkan
  per kategori, unit target tampil di samping input, beserta jumlah hasil.
- Di bawah 60×16, UI hanya menampilkan pemberitahuan "terminal too
  small" dengan logo ringkas.

Semua layar lain kini memakai satu kerangka yang sama: nav strip, tiles
ringkasan, tabel dengan masalah di atas, panel detail untuk baris terpilih
(layar penuh lewat `↵` di bawah 100 kolom), filter `/`, dan grup footer
sendiri. Tombol nav strip berpindah layar dari layar mana pun.

- Storage (`g`): tiles dari `podman system df`, lalu images, volumes, dan
  containers sebagai tab (`[ ]`). Kolom USED BY menautkan setiap objek ke
  unit quadlet yang memakainya; objek tak terpakai dikelompokkan di bawah
  UNUSED beserta ruang yang bisa di-reclaim. `p` pull, `D` hapus (ditolak
  selama dipakai unit), `P` prune, `S` urutkan berdasarkan ukuran atau nama.
- Events (`w`): satu timeline live yang menggabungkan `podman events`,
  pesan service manager tentang unit quadlet (started, failed, scheduled
  restart), dan aksi quadman sendiri, terbaru di atas. Filter tipe
  (`1`-`6`), unit (`u`), dan rentang waktu (`s`: 15m, 1h, 6h, 24h); `f`
  menjeda, `S` mengekspor TSV.
- Timers (`T`): jadwal dalam kata-kata ("daily 03:00", "every 1d"),
  next/last run beserta exit code, dan 24 run terakhir service yang
  dipicu sebagai strip ✓✗. `n` jalankan sekarang, `s`/`x` start/stop,
  `e`/`d` enable/disable, `l` log run terakhir, `E` edit berkas timer
  milik Anda.
- Secrets (`K`): USED BY · AS (env atau file target) dari setiap baris
  Secret=; secret yang hilang dan tidak terpakai naik ke atas. Nilai tidak
  pernah dirender: `n` membuat dan `r` merotasi lewat input bermasker yang
  dikirim ke podman via stdin (rotasi me-restart unit berjalan yang
  memakainya), `c` menyalin nilai dan membersihkan clipboard setelah 30
  detik (ditolak di sesi readonly dan serve), `D` menghapus setelah nama
  diketik.
- Actions (`A`): audit sesi mencatat durasi, cara aksi dipicu (tombol,
  palette, konfirmasi), dan konfirmasi yang dibatalkan; aksi gagal
  menampilkan error, perintah `systemctl` setara (`c` menyalinnya), dan
  masalah unitnya. `.` mengulang aksi setelah konfirmasi; `S` ekspor TSV.
- Stats (`o`): tiles host dengan sparkline CPU, bar CPU per container,
  urutan ←/→ berdasarkan CPU, memori, jaringan, nama, atau PIDS, container
  idle dikelompokkan di satu heading (`i`), dan sampling ulang setiap tick.
- Updates (`u`): tiles timer, next/last run, dan pending; satu baris per
  container dengan hasilnya (↑ pending, = current, ✓ updated, ✗ failed,
  ↺ rolled back). `a` menerapkan update sekarang setelah konfirmasi.
- Problems (`v`): setiap temuan dilokasikan di berkasnya dengan kutipan
  dan penanda `^`, plus langkah perbaikan bernomor ("did you mean
  pgdata.volume?"). `E` membuka editor di baris itu (vi, nano, emacs,
  helix, VS Code, dan lainnya).
- Tree (`t`): unit dikelompokkan per pod dan app podman, beserta
  referensi pod, image, network, volume, dan `[Unit]`; referensi yang
  hilang ditandai, subtree sehat dimulai terlipat (`←/→`, `z` semua), `f`
  hanya yang bermasalah, `i` menambah baris impact "blocked by / needed by".
- Detail: tab Overview baru (status, restart, result, boot, port, volume,
  secret, dependensi, masalah), header dengan uptime dan jumlah restart
  (petunjuk crash-loop mulai 3 restart), dan journal berkolom waktu ·
  sumber · pesan dengan level sebagai glyph + warna serta ID container
  dipersingkat. Nomor baris hanya di tab source.
- Generate (`n`): satu tab per berkas hasil, baris pemeriksaan (dry-run
  generator terhadap search path Anda, port host yang dipublikasikan unit
  lain, berkas yang sudah ada), serta toggle boot (`b`) dan start setelah
  ditulis (`space`).
- Konfirmasi berupa modal di atas layar yang mencantumkan apa yang WILL
  terjadi dan apa yang KEEPS, dengan fokus di Cancel. Bulk stop/restart
  mencantumkan unit dan unit yang bergantung padanya; delete (border err)
  baru aktif setelah nama unit diketik. `y` tetap mengonfirmasi dialog biasa.
- `quadman -version` mencetak logo blok beserta versi podman, systemd, Go,
  dan commit di terminal, dan satu baris versi biasa saat di-pipe.
- Logo baru (1a, "Quad Grid"): grid 2×2 dengan sel kanan-bawah sebagai
  unit yang Anda kelola, di samping wordmark half-block yang sama dengan
  splash. Header README memakainya sebagai SVG dengan varian terang dan
  gelap (`docs/logo/`); versi satu warna menggambar sel itu berongga.
  Logo ini menggantikan logo heksagon di header, beserta judul
  `### quadman` di bawahnya yang kini sudah terwakili oleh wordmark.

### Ditambahkan

- Kolom UPTIME, dari `ActiveEnterTimestamp` systemd (dibaca dengan
  `systemctl show --timestamp=unix`).
- Status systemd kini membawa `Result`, `ExecMainStatus`, dan `NRestarts`.

### Diperbaiki

- `podlet compose` menerima `-f`, yang ditolak podlet 0.3; berkas compose
  kini diberikan sebagai argumen posisional.
- Berkas compose dengan beberapa service tertulis ke satu berkas; setiap
  berkas hasil kini ditulis terpisah.
- `podman run -d …` yang ditempel gagal di podlet (butuh
  `--detach=true`); flag `-d`/`--detach` di awal kini ditulis ulang.
- Parsing `list-timers` memotong kolom rata kanan di offset header ("6
  days" menjadi "days", PASSED menjadi "in ago"); baris kini di-parse
  berdasarkan strukturnya.
- Error podman kini menyertakan penjelasan stderr dari podman sendiri.

## [v0.1.0-dev1] - 2026-10-05

Prarilis awal.

### Ditambahkan

- Logo proyek (`quadman_final_blue.png`) di header README.
- Changelog ini.

### Diubah

- Dokumen instalasi menunjuk ke aset prarilis `v0.1.0-dev1` via URL tag
  eksplisit (`/latest/` melewatkan prarilis).
- Tabel dukungan keamanan mengikuti `v0.1.0-dev1`; lini lama tidak didukung.
- Roadmap disusun ulang per tier, dan batch serve yang terkumpul
  dirilis sebagai `v0.1.0-dev1`.

### Diperbaiki

Batch hardening hasil audit menyeluruh:

- Klik mouse tidak lagi menggeser kursor saat prompt list
  (generate/instance/exec) sedang fokus.
- Layar updates memeriksa podman via session runner, sehingga sesi SSH
  dan kompartemen membaca podman remote.
- Nama file Quadlet hasil generate disanitasi ke base name dan mendapat
  akhiran `.container` hanya jika tanpa ekstensi.
- `serve` memberi peringatan saat config korup, bukan diam-diam memakai default.
- Tulis file atomik memakai temp file unik; ekspor log memakai pembuatan
  eksklusif (`O_EXCL`) dengan retry sufiks.
- Nilai `$EDITOR` berargumen (`code --wait`) dipecah quote-aware.
- Pencocokan tombol linger disederhanakan ke bentuk `L` / `shift+l`.
- Parsing `list-timers` memaksa `LC_ALL=C` (env lokal, prefix `env` saat remote).
- Health container cocok di mana saja dalam string status; filter list
  cocok per rune (aman non-ASCII).
- Aksi bulk melaporkan alasan gagal tiap unit (dibatasi, `+N more`).
- Runner sesi podman/podlet kini atomik (tanpa data race antara
  ganti sesi dan refresh yang sedang berjalan).
- Penanganan extra-dirs disatukan di `quadlet.AddExtraDirs`.
