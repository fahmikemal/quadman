# Panduan Kontribusi (Contributing)

<p align="left">
  <a href="CONTRIBUTING.md"><b>English</b></a> | <a href="CONTRIBUTING.id.md"><b>Bahasa Indonesia</b></a>
</p>

## Kebijakan Rilis (Release Policy)

quadman mengelompokkan perubahan ke dalam rilis yang bermakna (*meaningful releases*) — tidak ada penaikan versi untuk setiap commit kecil.

- **Kelompok Fitur (*Feature batch*)** (kemampuan baru, misal follow-mode logs, alur pengeditan):
  penaikan versi minor (`v0.N+1.0`).
- **Kelompok Perbaikan Bug (*Bugfix batch*)** (akumulasi beberapa perbaikan bug):
  penaikan versi patch (`v0.N.M+1`).
- **Commit docs / test / chore / kosmetik**: di-push langsung ke `main`, **tanpa tag**.

Dalam praktiknya: kerjakan perubahan di branch `main`, kumpulkan perubahan, dan saat perubahan tersebut dirasa cukup signifikan bagi pengguna, buat tag rilis `vX.Y.Z` — alur kerja rilis otomatis (`.github/workflows/release.yml` + GoReleaser) akan memublikasikan binary multi-arsitektur dan checksum secara otomatis.

## Aturan Repositori (House Rules)

- `make fmt vet test` wajib berstatus hijau (lulus) sebelum setiap push (divalidasi ketat oleh CI).
- Prefiks commit `docs:`, `test:`, `ci:`, dan `chore:` otomatis dikecualikan dari changelog yang dihasilkan (lihat `.goreleaser.yaml`).
- Fitur atau perilaku baru wajib disertai pengujian (*unit tests*); pembungkus CLI (*wrapper*) diuji menggunakan binary tiruan (lihat pola di `internal/systemd/systemd_test.go`).
- Perbarui tangkapan layar README setelah terjadi perubahan pada UI:
  `QUADMAN_SCREENSHOTS=1 go test ./internal/ui -run Screenshots`
