# e2e — PTY End-to-End Test untuk quadman TUI

<p align="left">
  <a href="README.md"><b>English</b></a> | <a href="README.id.md"><b>Bahasa Indonesia</b></a>
</p>

Menjalankan TUI asli di dalam pseudo-terminal (PTY): mengirimkan penekanan tombol nyata dan memverifikasi apa yang dirender di layar terminal, mencakup penemuan unit (discovery), filter fuzzy, start/stop, healthcheck, follow logs, enable/disable saat boot, layar auto-update, banner file usang (stale banner), daemon-reload, toggle linger pengguna, help overlay, dan tampilan berkas.

Modul ini sengaja dipisahkan (memiliki `go.mod` tersendiri) agar modul utama tetap bebas dari dependensi yang hanya dibutuhkan untuk tes, dan alur CI tidak menjalankannya secara default — modul ini memerlukan sesi pengguna (*user session*) systemd aktif, podman, dan unit-unit Quadlet di host.

## Menjalankan Tes

```sh
# dari root repositori, pastikan unit demo sudah terpasang (lihat di bawah)
go build -o /tmp/qe2e ./e2e
make build
cp quadman /tmp/quadman-under-test
cd /tmp && /tmp/qe2e
```

Harness pengujian mengeksekusi `./quadman` dari direktori kerjanya, jadi jalankan dari direktori yang memuat binary yang baru saja di-build (atau sesuaikan jalurnya di `main.go`).

## Fixture (Unit Uji Coba)

Pengujian membutuhkan tiga unit di `~/.config/containers/systemd/`:
`demo-web.container` (busybox dengan healthcheck dan `AutoUpdate=registry`),
`demo-data.volume`, dan `demo-net.network`. Harness akan memutasi statusnya (start/stop, menambah/menghapus `[Install]`) dan mengembalikan status boot ke kondisi semula di akhir pengujian; linger di-toggle dua kali dan dikembalikan seperti semula.
