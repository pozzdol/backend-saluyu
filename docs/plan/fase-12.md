# Fase 12 — Sederhana untuk Sehari-hari

Menjawab D3, D4, D11, D13, D16–D19, D21–D24, C11, C26 (lihat `masukan-putaran-3.md`, bagian 7).

## Definisi selesai
1. **Uang periode (D17):** setiap periode, `uang periode = uang yang bisa dibelanjakan di awal periode + pemasukan periode`. "Defisit" hanya bila pengeluaran melebihi uang periode. Akun baru dengan saldo > 0 dan tanpa pemasukan periode menampilkan sisa aman > 0 tanpa "Defisit", juga setelah satu jajan.
2. **Periode (C11, D5, D21):** gajian tanggal 1–31 (29–31 jatuh ke hari terakhir bulan pendek), hari kerja terakhir (Senin–Jumat terakhir), bulan kalender, mingguan (hari mulai dipilih).
3. **Kategori sistem (D19):** 5108 Bunga & Denda dan 5199 Beban Tunai Lain-lain tidak tampil sebagai ubin Catat cepat sampai punya minimal satu baris jurnal; tetap ada di form lengkap.
4. **Nama Indonesia (D3):** akun baru langsung bernama Tunai, Dompet Digital, Biaya Admin Bank. Akun lama yang namanya masih persis "Cash", "E-wallet", "Admin" mendapat banner "Ganti ke nama Indonesia?" (Daftar Akun dan Ringkasan) dengan satu tombol; nama yang sudah diubah tidak disentuh.
5. **Ringkasan (D4, D23):** "Total uangku" paling atas; kartu tanpa data (tabungan, anggaran, kesehatan, tren) diganti satu baris "muncul setelah …".
6. **Sembunyikan angka (D11):** tombol mata di menu menyamarkan semua jumlah (`Rp ••••••`), diingat di perangkat.
7. **Teks kecil:** judul daftar "Buat akunmu" / "Create your account" (D18); label "Kas & Bank" di daftar saldo dihapus (D22); "Catat kebalikannya" pindah ke menu "Lainnya" dengan penjelasan (D24); saldo awal tampil "Saldo awal" di daftar transaksi, nama ekuitas hanya di "Lihat jurnal" (C26); banner verifikasi bisa ditutup (D13); wizard memilih kategori yang dipakai (D16).
8. Navigasi tetap menu samping di HP. Mode gelap tidak termasuk.

## Database
**`000019_add_period_modes.up.sql`**
```sql
ALTER TABLE users DROP CONSTRAINT users_period_start_day_check;
ALTER TABLE users ADD CONSTRAINT users_period_start_day_check CHECK (period_start_day BETWEEN 1 AND 31);
ALTER TABLE users ADD COLUMN period_mode VARCHAR(16) NOT NULL DEFAULT 'payday'
    CHECK (period_mode IN ('payday', 'last_workday', 'calendar_month', 'weekly'));
ALTER TABLE users ADD COLUMN period_weekday SMALLINT NOT NULL DEFAULT 1
    CHECK (period_weekday BETWEEN 0 AND 6);
```
Down mengembalikan hari > 28 ke 28 lalu memasang lagi batas 1–28. Nomor 000020–000021 tidak dipakai.

## Backend
| Method | Path | Isi |
|---|---|---|
| GET/PATCH | `/me` | tambah `period_mode` (`payday` \| `last_workday` \| `calendar_month` \| `weekly`), `period_weekday` (0–6, 0 = Minggu); `period_start_day` kini 1–31 |
| GET | `/accounts` | tambah `has_entries` (akun punya minimal satu baris jurnal) |
| GET | `/dashboard` | tambah `opening_cash`, `period_money`, `cash_total` |
| GET | `/insights` | `safe_per_day` tambah `cash`, `scheduled`, `opening_cash`, `period_money`, `deficit`; `basis` kini `budget` atau `period_money` (dulu `surplus`) |

- **Periode:** `internal/period` punya `Config{Mode, Day, Weekday}`; `period.For(d, cfg)` dan `period.Back(p, k, cfg)` (mundur k periode, berlaku untuk semua mode). `LedgerRepository.PeriodStartDay` diganti `PeriodConfig`. Aturan berulang "payday" jatuh di hari pertama periode sesuai mode (mingguan → mingguan, hari kerja terakhir → pola baru `last_workday`).
- **Uang periode:** uang yang bisa dibelanjakan = Kas & Bank, tunai, dompet digital (aset selain Piutang 1101, Investasi, dan kategori Tabungan). Saldo dihitung menurut tanggal periode, jadi gaji yang masuk lebih awal untuk periode ini terhitung sekali sebagai pemasukan. Saldo awal yang diisi di dalam periode dihitung sebagai uang awal periode.
- **Sisa aman per hari** = (uang yang ada sekarang − utang kartu − cicilan belum dibayar s.d. akhir periode − pembayaran berulang terbuka s.d. akhir periode) ÷ sisa hari, minimal 0. Dengan anggaran: sisa anggaran, tetapi tidak lebih dari angka tadi.
- **Seed:** `DefaultAccounts` memakai Tunai, Dompet Digital, Biaya Admin Bank. Tidak ada migration ganti nama: pengguna lama memilih lewat banner.

## Frontend
- `components/period-picker.tsx`: `PeriodFields` (mode, tanggal 1–31, hari mulai minggu) dipakai di Profil dan wizard; `fmtPeriod` menulis bulan kalender sebagai "Oktober 2026" dan minggu sebagai "19 – 25 Okt 2026".
- `lib/utils.ts`: `amountsHidden`/`setAmountsHidden` (localStorage `saluyu.hideAmounts`, muat ulang halaman), `fmtIDR`/`fmtRupiah` menyamarkan; `nextPeriodStart` (dipakai form transaksi untuk tawaran "hitung ke periode berikutnya").
- Ringkasan: kartu Total uangku, ubin "Sisa uang periode" (uang periode − pengeluaran), baris `Soon` untuk kartu kosong, banner nama Indonesia.
- Catat cepat menyembunyikan 5108/5199 bila `has_entries` false. Wizard langkah 4: chip kategori; yang tidak dicentang diarsipkan (`is_active = false`).
- Detail transaksi: "Lainnya" (`<details>`) berisi penjelasan dan tombol "Catat kebalikannya".
- Banner verifikasi: tombol tutup, diingat per email (`saluyu.verifyDismissed`).
- i18n: kunci baru di blok `f12` di akhir `id.ts` dan `en.ts`.

## Cara uji
- Backend: `go vet ./... && go test ./...` (`internal/period/period_test.go` `TestForModes`; `internal/handler/phase12_test.go`: buku baru, sisa terbawa tanpa pemasukan, defisit lewat kartu, berulang dan kartu mengurangi sisa aman, mode periode lewat PATCH `/me`, nama seed dan `has_entries`).
- Frontend: `node --experimental-strip-types src/lib/i18n.test.ts`, `node --experimental-strip-types src/lib/utils.test.ts` (termasuk `nextPeriodStart`), `npx tsc --noEmit -p tsconfig.app.json`, `npx vite build`.
- Manual: daftar akun baru, isi saldo, catat jajan → Ringkasan tanpa "Defisit", sisa aman > 0; Profil → pilih "Mingguan" → pemilih periode menampilkan satu minggu.

## Keputusan
- **Sisa aman memakai uang yang ada, bukan "uang periode − pengeluaran".** Rumus bagian 7 menghitung pembelian kartu dua kali (sebagai pengeluaran lalu sebagai tagihan kartu) dan membuat angka naik setelah cicilan dibayar (pokok bukan pengeluaran). Dengan uang yang ada: bayar tagihan → uang dan tagihan turun bersama; belanja kartu → terhitung sekali sebagai utang kartu. Uang periode tetap dipakai untuk "Sisa uang periode" dan penanda defisit.
- **Seluruh utang kartu** dikurangkan, bukan hanya tagihan yang jatuh tempo di periode ini: lebih hati-hati, dan tidak perlu memisahkan belanja per siklus.
- **Tabungan (kategori Tabungan) tidak termasuk** uang untuk dibelanjakan; tetap masuk "Total uangku".
- **Kategori yang tidak dipilih di wizard diarsipkan** (`is_active`), tanpa kolom baru; bisa diaktifkan lagi di Daftar Akun.
- **Hari libur nasional** belum dihitung untuk "hari kerja terakhir".
- **Sembunyikan angka** memuat ulang halaman agar semua angka tergambar ulang, alih-alih setiap halaman berlangganan state.
- Aturan berulang pemasukan tidak ditambahkan ke sisa aman: gaji yang baru dijadwalkan belum menjadi uang.
