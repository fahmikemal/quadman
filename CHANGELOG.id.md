# Changelog

> English: [CHANGELOG.md](CHANGELOG.md).

Semua perubahan penting quadman dicatat di sini. Format mengikuti
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Diperbaiki

- Pindah ke Updates, Storage, Timers, Secrets, atau Stats tidak lagi
  memunculkan sekilas dashboard daftar unit saat data dimuat: layar terbuka
  begitu tombol ditekan, dengan data terakhirnya atau tampilan memuat.
- Data layar yang datang setelah layar itu ditinggalkan tidak lagi menarik
  kembali ke layar tersebut (dulu, keluar dari Stats saat sampel sedang
  diambil membuka Stats lagi).
- Membuat, me-rotate, atau menghapus secret serta menghapus storage memuat
  ulang daftar setelah aksinya selesai, bukan bersamaan, sehingga daftar
  menampilkan hasilnya.
- Membuka layar dari journal atau tampilan events (palette, tombol nav)
  menghentikan stream `journalctl -f` / `podman events`-nya.
- Berganti sesi (kompartemen) membuang data layar dari target sebelumnya.
- Perpindahan antar layar jadi cepat:
  - data yang dibuka lagi dalam 30 detik (updates: 5 menit) langsung
    ditampilkan beserta umurnya; `r` memuat ulang;
  - load yang ditinggalkan dibatalkan, sehingga proses podman-nya berhenti
    alih-alih menumpuk di belakang layar berikutnya;
  - storage, timers, secrets, updates, dan stats menjalankan pembacaan yang
    saling lepas secara paralel (storage: lima panggilan podman sekaligus);
  - stats mengambil satu sampel dalam satu waktu dan tidak lagi menjalankan
    `podman info` dan `podman ps` di setiap tick.

### Diubah

- quadman yang dibiarkan terbuka jauh lebih hemat CPU: setelah satu menit
  tanpa input, atau saat terminal tidak fokus, daftar unit di-poll setiap
  15 detik, bukan 2,5 detik (top bar menampilkan `15s idle`); tombol, klik,
  atau fokus berikutnya langsung memperbarui. `refresh_interval` yang lebih
  lambat tetap dipakai.
- Linger dibaca setiap 12 poll (serta saat refresh penuh dan ganti sesi),
  bukan di setiap poll; `L` melaporkan hasilnya sendiri.

## [v0.7.0-beta.1] - 2026-10-12

Beta pertama. Semua versi sebelumnya (`v0.1.0` sampai `v0.6.0`, dan
`v0.1.0-dev1`) ditarik (retract) di `go.mod`: kodenya lebih lama dari
lini rilis ini, dan proxy modul Go tetap menyimpannya, sehingga lini ini
berlanjut di atas nomor-nomor itu alih-alih memakainya ulang.

### Keamanan

- **Sesi serve terisolasi.** Sesi `quadman serve` dulu berbagi runner
  podman/podlet dan style UI: satu klien yang berpindah kompartemen
  mengarahkan panggilan podman semua sesi lain (secrets, storage, prune)
  ke user lain, dan koneksi bersamaan saling race di style. Kini setiap
  sesi punya client sendiri, dan tampilan diset sekali sebelum sesi
  pertama.
- Klien `quadman --as <user> serve` bisa keluar dari kompartemen ke sesi
  operator ("Use Own Session" selalu aktif, bahkan saat readonly) atau
  pindah ke kompartemen lain. Sesi serve kini tidak bisa pindah sesi sama
  sekali.
- Auth password hanya loopback meski ada `--authorized-keys` (bind LAN
  dengan keduanya dulu menerima password dari jaringan), dan percobaan
  password gagal dibatasi per alamat klien.
- Mode readonly tidak lagi membiarkan `I` menyiapkan install bundel. Handler
  tombol dan palette kini berbagi satu kebijakan kapabilitas, yang juga
  mengakhiri perbedaan palette/tombol untuk exec dan prune.
- Mengulang aksi (`A`, `.`) dulu cukup `y` bahkan untuk aksi yang awalnya
  dikonfirmasi dengan mengetik nama (delete, hapus secret, prune) dan bisa
  diputar ulang setelah pindah sesi ke host atau user lain. Kini repeat
  meminta konfirmasi aslinya dan ditolak lintas sesi.
- Menghapus volume dan `system prune` kini wajib mengetik namanya (atau
  `prune`), seperti menghapus unit.
- Custom command dulu mengekspansi template sebelum dipecah, sehingga nilai
  dari file unit (Image=…) bisa menambah argumen; kini dipecah dulu dan
  setiap kata diekspansi sendiri. Perintah juga berjalan lewat runner sesi,
  bukan selalu di host lokal.
- Sudo kompartemen membawa variabel sesi sebagai argumen `VAR=value` sudo,
  sehingga sudoers tidak lagi harus mengizinkan `/usr/bin/env` (yang
  membolehkan perintah apa pun); probe memakai bentuk yang sama.
- config.yaml yang tidak bisa dipakai (rusak atau key tak dikenal)
  membuat TUI mulai readonly dan menghentikan `serve`; config.json yang
  rusak tidak lagi menghapus pengaturan YAML (termasuk `readonly: true`).

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
- Perilaku `--ssh`: perintah remote di-quote untuk shell POSIX remote
  (OpenSSH mem-parse ulang baris perintah); pemanggilan berbagi satu
  koneksi multiplex (`ssh_multiplex: false` untuk menonaktifkan);
  instansiasi template, install bundel, dan prune kini berjalan via SSH;
  `quadman --ssh host list` menampilkan unit host. `--ssh` dengan `--as`,
  dan `serve` dengan `--ssh`, ditolak.
- `quadman --as <user>` berhenti saat startup bila kompartemen tidak bisa
  dijangkau (seperti `list` dan `serve`), alih-alih membuka sesi operator.
  Operator root boleh memakai `--as`; kompartemen selalu sesi user, juga
  bila config.yaml berisi `system: true`.
- Discovery memindai subdirektori setiap search path seperti generator;
  direktori dan berkas symlink diikuti.
- Nama section dan key bersifat case-sensitive, seperti systemd dan Quadlet.
- Enable saat boot memakai `multi-user.target` untuk unit sistem, menambah
  key ke section `[Install]` yang sudah ada, dan menghormati
  `RequiredBy=`/`UpheldBy=`; disable mempertahankan key `[Install]` lain dan
  menolak (dengan path-nya) bila unit di-enable oleh drop-in.
- Berkas hasil generate ditulis ke `/etc/containers/systemd` di mode sistem,
  hanya ekstensi Quadlet yang diterima dari `# FileName=` podlet, dan start
  setelah menulis menghormati `ServiceName=`.
- Start/stop/restart menunggu hingga 5 menit; job yang masih dijalankan
  systemd setelahnya dilaporkan sebagai pending, bukan gagal.
- Layar timers membaca nama dari `list-units --plain` dan waktu dari `show
  --timestamp=unix`, bukan tabel `list-timers` yang dilokalisasi.
- Level di ekspor log dicocokkan per kata utuh ("interrupt" tidak lagi
  dianggap error).
- CI mem-pin govulncheck dan gosec, mengacak urutan tes race, dan
  mem-build modul e2e.

### Ditambahkan

- Kolom UPTIME, dari `ActiveEnterTimestamp` systemd (dibaca dengan
  `systemctl show --timestamp=unix`).
- Status systemd kini membawa `Result`, `ExecMainStatus`, dan `NRestarts`.
- `NeedDaemonReload` di status (menggerakkan banner stale-reload),
  pengaturan `ssh_multiplex`, `quadman --ssh host list`, dan `DESIGN.md`.

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
- `podman quadlet list --format` terpecah oleh shell remote via SSH,
  sehingga discovery jatuh ke daftar semua user service (dbus, pipewire, …)
  sebagai "container"; fallback tanpa podman kini hanya menyimpan unit yang
  `SourcePath=`-nya berkas Quadlet.
- Linger via SSH menanyakan nama user lokal di host remote, dan di
  kompartemen memakai linger operator; keduanya kini memakai user sesi itu
  sendiri.
- `sudo quadman --as <user>` gagal dengan "--system and --as cannot be
  combined"; `quadman --ssh host list` diam-diam menampilkan unit lokal.
- Banner stale-reload selalu muncul di kompartemen (operator tidak bisa
  stat runtime dir user lain); deteksi stale kini memakai `NeedDaemonReload`
  systemd, benar untuk sesi lokal, SSH, dan kompartemen. Validasi dan cek
  secret juga berjalan lewat sesi.
- Health, healthcheck, inspect, events, dan stats menebak container sebagai
  `systemd-<nama file>`, meleset untuk `ContainerName=` dan `ServiceName=`;
  kini label `PODMAN_SYSTEMD_UNIT` (lalu `ContainerName=`, lalu `systemd-%N`)
  menamainya di semua tempat.
- Enable/disable saat boot mengganti quadlet yang di-symlink menjadi berkas
  biasa, melepasnya dari checkout git; penulisan kini mengikuti link,
  fsync, dan mempertahankan mode (serta owner sebagai root).
- Tick poll menumpuk refresh pada sesi lambat; kini hanya satu refresh poll
  berjalan, setiap refresh dibatasi waktunya, dan hasil dari sesi lama atau
  yang datang tidak berurutan dibuang.
- Hapus unit memberi path ke `podman quadlet rm`, yang menerima nama; kini
  hanya dipakai bila podman melaporkan berkas yang persis sama.
- Layar secrets: `Secret = x` (dengan spasi) dan referensi di drop-in kini
  ditemukan.
- Dua tes bergantung pada `TERM` dan `/run/user/<uid>`; gosec G115 di
  decoder byte-array journal.
- `DESIGN.md`, yang dirujuk kode dan changelog ini, kini ada di repositori.

## [v0.1.0-dev1] - 2026-10-05 [YANKED]

Prarilis awal, ditarik; seluruh isinya ikut dirilis di `v0.7.0-beta.1`.

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
