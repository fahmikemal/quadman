# Kebijakan Keamanan

> English: [SECURITY.md](SECURITY.md).

## Versi yang didukung

| Versi   | Didukung            |
|---------|---------------------|
| 0.5.x   | Ya (terbaru: 0.5.0) |
| < 0.5.0 | Best effort — mohon upgrade dan uji ulang dulu |

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

- `quadman serve` default ke loopback (`127.0.0.1:2222`); setiap
  bind non-loopback **menolak berjalan** tanpa `--authorized-keys`,
  sehingga bind open-auth dan password-only hanya boleh di loopback
  (`internal/server/server.go`, ditegakkan di `Validate`, fail-fast di `main.go`).
- `quadman serve` tanpa `--authorized-keys` maupun `--password`
  **memaksa mode readonly** plus peringatan keras; tulis anonim tidak
  pernah aktif secara default (`main.go`).
- Password sesi serve dibandingkan secara constant-time
  (`internal/server/server.go`).
- Password serve bisa berasal dari `--password-file`, `QUADMAN_SERVE_PASSWORD`,
  atau config `password_file`, sehingga secret tidak muncul di process list;
  tepat satu sumber password boleh diisi (`main.go`).
- Direktori host-key dibuat `0700`; `config.json` ditulis `0600`
  (`internal/server`, `internal/config`).
- `config.yaml` berisi password serve yang terbaca selain owner
  memicu peringatan `chmod 600` saat startup (`main.go`).
- Eksekusi remote memakai binary `ssh` milik user dengan
  `BatchMode=yes` dan connect timeout, sepenuhnya argv-based (tanpa
  shell string remote); kompartemen sudo memakai `sudo -n`
  non-interaktif (`internal/remote`).
- Setiap panggilan `systemctl`/`journalctl` dibatasi (default 30 dtk),
  panggilan podman/SSH dibatasi (default 10 dtk).
- Listen di semua interface wajib `--authorized-keys` (ditolak
  tanpanya) dan dengan akses tulis tetap memicu peringatan; gunakan
  `-a 127.0.0.1:2222` atau `--readonly`.
- Sesi terisolasi (target SSH, kompartemen `--as`, sesi serve)
  menolak tulis file lokal, sudo hop bersarang, dan pembajakan proses
  `$EDITOR`, dengan penjelasan di UI.

## Supply chain

CI mewajibkan di setiap push/PR ke `main`: `go build`, `go vet`,
`go test -race`, `gofmt` bersih, `govulncheck`, `gosec`, plus CodeQL
mingguan dan dependency review (lihat `.github/workflows/`).

## Ruang lingkup

Dalam lingkup: CLI quadman, TUI, dan daemon `serve` di repo ini.
Di luar lingkup: kerentanan di Podman, systemd, atau OS itu sendiri —
tetapi laporan tentang cara quadman salah menangani output mereka
tetap diterima.
