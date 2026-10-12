# Kebijakan Keamanan

> English: [SECURITY.md](SECURITY.md).

## Versi yang didukung

| Versi       | Didukung            |
|-------------|---------------------|
| v0.1.0-dev1 | Ya (terbaru, prarilis) |
| lama        | Tidak (mohon upgrade ke rilis terbaru) |

Perbaikan keamanan dirilis sebagai patch di `main` dan dipublikasikan
melalui GitHub Releases beserta checksum (lihat `.github/workflows/release.yml`).

## Melaporkan kerentanan

Mohon **jangan** membuka issue publik berisi detail eksploitasi. Gunakan
jalur privat repo ini: **Security → Report a vulnerability** (membuat
Security Advisory privat).

Sertakan:

- versi quadman (`quadman -version`) dan sumber instalasi (tarball rilis, `go install`, build dari source),
- mode operasi (`--system`, `--ssh`, `--as`, `serve`, atau TUI lokal),
- langkah reproduksi dan dampak yang terlihat,
- hal yang sudah Anda singkirkan.

Coordinated disclosure sangat dihargai: beri kesempatan maintainer
merilis perbaikan sebelum detail dipublikasikan.

## Default hardening (sudah diimplementasikan)

Ini terimplementasi di kode, bukan sekadar anjuran:

- **Satu kebijakan kapabilitas.** Setiap aksi berhak-istimewa menyatakan
  kebutuhannya (ubah state, tulis file quadlet, tulis file lokal,
  terminal, nilai secret, pindah sesi); `internal/ui/policy.go`
  memutuskan per jenis sesi (lokal, `--ssh`, `--as`, serve, `--system`)
  dan `--readonly`. Handler tombol dan Command Palette memakai tabel
  yang sama, jadi keduanya tidak pernah berbeda dan aksi baru tidak bisa
  melewati gate.
- `quadman serve` default ke loopback (`127.0.0.1:2222`); setiap bind
  non-loopback **menolak berjalan** tanpa `--authorized-keys`. Auth
  password **hanya loopback, bahkan bila ada keys**: bind LAN hanya
  memakai keys (`internal/server/server.go`, ditegakkan di `Validate`,
  fail-fast di `main.go`).
- Percobaan password gagal dibatasi per alamat klien (jeda di setiap
  kegagalan, lockout setelah 5 kegagalan dalam semenit); password
  dibandingkan secara constant-time.
- `quadman serve` tanpa `--authorized-keys` maupun `--password`
  **memaksa mode readonly** plus peringatan keras, termasuk bahwa
  loopback bukan batas antar-user di host bersama; tulis anonim tidak
  pernah aktif secara default (`main.go`).
- **Sesi serve terisolasi satu sama lain.** Setiap sesi punya client
  systemctl/loginctl/podman/podlet sendiri; tidak ada aksi satu sesi
  yang mengarahkan ulang sesi lain. Tampilan (tema, glyph) diset sekali
  per proses sebelum sesi pertama. Sesi serve tidak pernah bisa pindah
  kompartemen atau keluar dari kompartemen `quadman --as <user> serve`
  ke sesi operator sendiri, dan tidak pernah menjalankan editor, shell,
  ekspor file lokal, atau salin nilai secret.
- `quadman serve` menolak berjalan bila `config.yaml` tidak bisa dipakai
  (rusak atau key tak dikenal) alih-alih melayani default tebakan; TUI
  mulai dalam mode **readonly** untuk kasus itu. `config.json` yang
  rusak (preferensi editor) tidak pernah menghapus pengaturan YAML.
- Password serve bisa berasal dari `--password-file`, `QUADMAN_SERVE_PASSWORD`,
  atau config `password_file`, sehingga secret tidak muncul di process list;
  tepat satu sumber password boleh diisi (`main.go`).
- Direktori host-key dibuat `0700`; `config.json` ditulis secara atomik
  dengan mode `0600` (`internal/server`, `internal/config`).
- `config.yaml` berisi password serve yang terbaca selain owner
  memicu peringatan `chmod 600` saat startup (`main.go`).
- Eksekusi remote memakai binary `ssh` milik user dengan
  `BatchMode=yes` dan connect timeout. OpenSSH mem-parse ulang baris
  perintah di sisi remote, jadi setiap kata di-quote tunggal POSIX: host
  remote menjalankan argv persis seperti yang dibangun quadman (login
  shell remote harus kompatibel POSIX). Multiplexing koneksi memakai
  direktori control-socket privat `0700` yang dihapus saat keluar.
- Kompartemen sudo memakai `sudo -n -u <user> VAR=value -- <cmd>`
  non-interaktif: variabel sesi dibawa sebagai argumen sudo sendiri,
  sehingga aturan sudoers bisa mengizinkan tepat CLI yang dijalankan
  quadman (dengan `SETENV`) alih-alih `/usr/bin/env` yang terbuka.
  Probe saat startup memakai bentuk yang sama, jadi `SETENV` yang
  kurang langsung ketahuan.
- Aksi destruktif mengonfirmasi apa yang akan terjadi dan apa yang
  tetap. Hapus unit, hapus volume, hapus secret, dan `system prune` wajib
  mengetik namanya (atau `prune`). Mengulang aksi dari layar actions
  meminta konfirmasi yang sama lagi dan ditolak setelah pindah sesi.
- Custom command dipecah menjadi kata sebelum ekspansi template, jadi
  nilai dari file unit tidak pernah bisa menambah atau memecah argumen;
  dijalankan lewat runner sesi, tidak pernah lewat shell.
- Edit file quadlet bersifat atomik (file temp, `fsync`, rename, `fsync`
  direktori), mengikuti symlink untuk mengganti file aslinya, dan
  mempertahankan mode-nya (serta owner bila berjalan sebagai root).
- Setiap panggilan `systemctl`/`journalctl` dibatasi (default 30 dtk;
  job lifecycle menunggu hingga 5 menit dan melaporkan job yang masih
  dijalankan systemd sebagai pending), panggilan podman/SSH dibatasi
  (default 10 dtk), dan satu refresh dibatasi (90 dtk) dengan paling
  banyak satu refresh poll berjalan.

## Supply chain

CI mewajibkan di setiap push/PR ke `main`: `go build`, `go vet`,
`go test -race -shuffle=on`, `gofmt` bersih, golangci-lint di kedua
modul, serta scan `govulncheck` dan `gosec` yang versinya di-pin (agar
rilis scanner tidak membuat CI merah dengan sendirinya; dinaikkan secara
sengaja), plus CodeQL dan dependency review (lihat `.github/workflows/`).

## Ruang lingkup

Dalam lingkup: CLI quadman, TUI, dan daemon `serve` di repo ini.
Di luar lingkup: kerentanan di Podman, systemd, atau OS itu sendiri —
tetapi laporan tentang cara quadman salah menangani output mereka
tetap diterima.
