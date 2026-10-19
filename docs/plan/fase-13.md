# Fase 13 — Pengingat Email

Spesifikasi: `masukan-putaran-3.md` bagian 7. Web Push, bot, dan ketik bebas tetap ditunda; fase ini hanya email lewat Resend yang sudah ada.

## Definisi selesai
1. **Empat jenis pengingat**, masing-masing bisa dimatikan di Pengaturan (bawaan: semua aktif). Hanya email yang sudah terverifikasi yang menerima.
   - `due_bills`: transaksi berulang (selain pemasukan), cicilan berikutnya, dan tagihan kartu yang belum lunas, dikirim **H-3** dan **hari-H**.
   - `budget`: kategori yang melewati **80%** anggaran periode berjalan, dan lagi saat melewati **100%**.
   - `weekly`: tiap **Senin**: pengeluaran 7 hari terakhir, sisa aman per hari, 3 kategori terbesar.
   - `inactive`: transaksi terakhir sudah **3 hari atau lebih**; sekali per jeda (kunci = tanggal transaksi terakhir). Pengguna yang belum pernah mencatat tidak dikirimi.
2. **Tiap kejadian terkirim sekali.** Job bisa berjalan ulang (atau dua instance sekaligus) tanpa kirim dobel; jenis yang dimatikan tidak mengirim apa pun.
3. **Rangkaian hari (streak)** di Ringkasan: `streak_days` = hari berturut-turut (berakhir hari ini atau kemarin) dengan minimal satu transaksi yang dicatat pengguna.
4. **Email dua bahasa** mengikuti `locale` pengguna (kosong = Indonesia), HTML + teks polos.

## Database
**`000022_create_notifications.up.sql`**
- `notification_prefs(user_id PK, due_bills, budget, weekly, inactive, updated_at)`: tanpa baris = semua aktif.
- `notification_log(user_id, kind, ref, sent_on, PRIMARY KEY (user_id, kind, ref))`: indeks unik inilah yang mencegah kirim ganda.

Kunci `ref`: `due:<id aturan>:<tanggal>:h3|h0`, `due:<id cicilan>:<seq>:<tanggal>:h3|h0`, `due:<id kartu>:<jatuh tempo>:h3|h0`, `budget80:<id akun>:<awal periode>`, `budget100:<id akun>:<awal periode>`, `weekly:<tanggal>:<n>`, `inactive:<tanggal transaksi terakhir>`.

## Backend
| Method | Path | Isi |
|---|---|---|
| GET | `/me/notifications` | `{due_bills, budget, weekly, inactive}` |
| PUT | `/me/notifications` | keempat boolean wajib ada; mengembalikan nilai tersimpan |
| GET | `/dashboard` | ditambah `streak_days` |

- **`service.NotifyService`** (`internal/service/notify.go`): `Run(ctx, now)` mengevaluasi tiap jenis untuk tiap pengguna terverifikasi; `now` disuntikkan, jadi bisa diuji. Memakai `ReportService.Insights` (peringatan anggaran, sisa aman), `RecurringService`, `CreditService`, dan `EntryDays`; tidak menghitung ulang rumus apa pun.
- **Klaim lalu kirim:** tiap pengingat di-`Claim` (INSERT ... ON CONFLICT DO NOTHING) sebelum email dikirim; satu email per jenis berisi semua butir yang baru. Bila pengiriman gagal, klaim dikembalikan (`Unclaim`) supaya dicoba lagi jam berikutnya.
- **Penjadwal:** `NotifyService.StartJob(ctx, time.Hour)` dipanggil di `cmd/api/main.go` (pola sama dengan `RecurringService.StartJob`). Tidak ada pengiriman sebelum pukul 07:00 waktu server.
- **Email:** `mail.Digest` + `internal/mail/digest.html` (daftar butir + satu tombol), teks per bahasa di `notifyTexts`.
- Rute didaftarkan lewat `handler.RegisterNotifyRoutes` (tanpa mengubah tanda tangan `RegisterRoutes`).

## Frontend
- Halaman Akun (`/account`): bagian "Pengingat email" dengan empat sakelar; catatan bila email belum terverifikasi (pengingat tidak dikirim).
- Ringkasan: satu baris kecil "n hari berturut-turut mencatat" bila `streak_days` > 0.

## Test
- `internal/handler/phase13_test.go`: `TestEmailReminders` (job dua kali = satu email per jenis, tidak kirim sebelum 07:00, jenis yang dimatikan tidak mengirim lalu mengirim saat dinyalakan, `streak_days`, GET/PUT pengaturan).

## Batasan (ponytail)
- "Hari ini" mengikuti kalender server, bukan zona waktu pengguna.
- Streak menghitung semua transaksi selain saldo awal, hitung kas, dan penyesuaian; transaksi berulang otomatis ikut dihitung. Batas lihat ke belakang 1 tahun.
- Satu interval jam: email dikirim pada run pertama setelah pukul 07:00.
