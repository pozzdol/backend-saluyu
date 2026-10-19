# Fase 15 — Akun Lebih Tepat

Menjawab C22 dan C23 (lihat [masukan-putaran-3.md](masukan-putaran-3.md), bagian 7). **Fase 15 adalah fase terakhir yang direncanakan.**

## Definisi selesai
1. **Bunga dipisah (C23):** "Bunga & Denda" (5108) tidak lagi menampung semuanya. Akun bawaan baru: **5111 Bunga Pinjaman** (bunga angsuran/pinjaman) dan **5112 Bunga & Denda Kartu** (bunga dan denda kartu/paylater). **Biaya bank memakai 5107 Admin** yang sudah ada (biaya admin transfer), jadi tidak dibuat akun ketiga. 5108 tetap, diganti nama **"Denda & Bunga Lain"** untuk sisanya.
2. **Aset tetap (C22):** kategori akun `Aset Tetap` dengan 1301 Kendaraan, 1302 Properti, 1303 Elektronik. Pencairan pinjaman bisa langsung menambah aset ("kredit motor menambah Kendaraan, bukan Bank"). Nilainya diperbarui manual; selisihnya masuk ekuitas **3002 Penyesuaian Nilai Aset**.
3. Aset tetap muncul di neraca dan ikut total kekayaan bersih, tetapi **tidak** dihitung sebagai uang yang bisa dipakai: tidak ada di saldo Ringkasan, tidak di `isCash` (insight, arus kas), tidak ditawarkan sebagai sumber/tujuan pemasukan dan pengeluaran harian.

## Database
**`000026_split_interest_accounts`**
- Menambah 5111 dan 5112 untuk semua pengguna (`ON CONFLICT DO NOTHING`; bila kode itu sudah dipakai akun lain, pemindahan dilewati karena dicocokkan juga lewat nama).
- Memindahkan baris jurnal di 5108:
  - entry `kind = 'installment'` → 5111;
  - entry lain (bukan angsuran) yang menyentuh akun `credit_card`/`paylater` → 5112;
  - sisanya tetap di 5108.
- `installment_plans.interest_account_id` yang menunjuk 5108 dialihkan ke 5111 (angsuran berikutnya memakai akun baru).
- 5108 diganti nama "Denda & Bunga Lain" hanya bila namanya masih "Bunga & Denda".
- Down: semua baris dan rencana dikembalikan ke 5108, nama dikembalikan, 5111/5112 dihapus bila tak dipakai lagi.

**`000027_add_fixed_assets`:** menambah 1301–1303 (kategori `Aset Tetap`) dan 3002 untuk semua pengguna. Down menghapus akun-akun itu bila belum punya baris jurnal. Tidak ada perubahan skema.

Akun bawaan pengguna baru ada di `model.DefaultAccounts` (dijaga sama dengan kedua migration).

## Backend
| Method | Path | Isi |
|---|---|---|
| POST | `/accounts/:id/revalue` | `{value (sen), entry_date}` → entry `adjustment`: nilai naik = Dr aset / Cr 3002, turun = kebalikannya. Nilai sama → 400 `no_value_change`; bukan aset tetap → 400 `not_fixed_asset`. |
| POST | `/installments` | menerima `asset_account_id` (opsional, harus aset tetap): uang pinjaman didebit ke aset itu, `received_into_id` diabaikan. |

- Bunga angsuran default ke 5111 (`interestAccount` di `credit.go`).
- `fits()` (form sederhana): aset tetap bukan sumber pengeluaran/transfer dan bukan tujuan pemasukan. Boleh jadi tujuan **transfer** (membeli aset tunai dari bank).
- `Account.IsFixedAsset()` dipakai `isCash`, saldo Ringkasan, dan `fits()`.
- Bunga/denda kartu belum punya form khusus: pengguna memilih 5112 sebagai kategori pengeluaran dari kartu.

## Frontend
- **Daftar Akun:** kelompok "Aset Tetap" terpisah dari Aset, menampilkan nilai hari ini (dari neraca saldo) dan tombol "Perbarui nilai" (sheet: nilai baru + tanggal).
- **Cicilan baru:** pilihan "Pinjaman ini untuk membeli aset" (muncul bila ada aset tetap aktif); bila dipilih, pilihan "Uang masuk ke" disembunyikan.
- `isMoney`, Catat cepat, dan form transaksi tidak menawarkan aset tetap. Ikon: car, house, tv (sudah ada).

## Test
`TestLoanForFixedAsset`: pencairan ke Kendaraan (bank tetap 0), angsuran memposting bunga ke 5111, aset tetap ditolak di form harian, revaluasi naik/turun menghasilkan entry seimbang, neraca tetap seimbang dan memuat Kendaraan, saldo Ringkasan tidak memuat Kendaraan.

## Di luar fase ini
Penyusutan otomatis, riwayat nilai pasar, dan form khusus bunga kartu belum dibuat. Fase 15 menutup rencana putaran 3.
