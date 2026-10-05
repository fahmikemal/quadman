# Changelog

> English: [CHANGELOG.md](CHANGELOG.md).

Semua perubahan penting quadman dicatat di sini. Format mengikuti
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [v0.1.0-dev1] - 2026-10-05

Rilis pertama setelah penomoran versi dimulai ulang: semua tag dan rilis
lama telah dihapus dan penomoran dimulai lagi dari sini sebagai prarilis.

### Ditambahkan

- Logo proyek (`quadman_final_blue.png`) di header README.
- Changelog ini.

### Diubah

- Dokumen instalasi menunjuk ke aset prarilis `v0.1.0-dev1` via URL tag
  eksplisit (`/latest/` melewatkan prarilis).
- Tabel dukungan keamanan mengikuti `v0.1.0-dev1`; lini lama tidak didukung.
- Roadmap disusun ulang per tier: label versi lama hanya arsip pra-reset,
  dan batch yang terkumpul dirilis sebagai `v0.1.0-dev1`.

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
