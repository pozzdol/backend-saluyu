# Fase 14 — Anggaran dan Ekspor

Menjawab bagian "Fase 14" di [masukan-putaran-3.md](masukan-putaran-3.md) (bagian 7): anggaran amplop, kelompok 50/30/20, ekspor transaksi (C24) dan saldo berjalan (C24).

## Definisi selesai
1. **Amplop per kategori:** tiap kategori pengeluaran punya sakelar "Bawa sisa ke periode berikutnya". Bawaan mati: anggaran reset tiap periode seperti sebelumnya. Bila hidup, dana tersedia periode ini = anggaran + sisa terkumpul (anggaran − terpakai) dari periode-periode sebelumnya. Sisa boleh negatif: belanja berlebih mengurangi periode berikutnya.
2. **50/30/20:** kategori bawaan sudah punya kelompok (Kebutuhan: Transportasi, Makan, Keluarga, Admin; Keinginan: Jajan, Hiburan; Tabungan: akun Tabungan dan Investasi). Pengguna bisa memindahkan atau mengosongkan kelompok di halaman Anggaran. Kartu "Kebutuhan, keinginan, tabungan" membandingkan bagian nyata dari pemasukan periode dengan 50/30/20.
3. **Ekspor:** `GET /entries/export?format=csv|xlsx` menerima filter yang sama persis dengan daftar transaksi. CSV UTF-8 dengan BOM (terbuka benar di Excel). Dua tombol di halaman Transaksi meneruskan filter yang aktif.
4. **Saldo berjalan:** bila daftar difilter satu akun, tiap baris membawa `balance_after`; kolom "Saldo" tampil hanya saat filter itu aktif, dan ikut di ekspor.

## Database
**`000024_add_budget_carry_and_groups.up.sql`**
```sql
ALTER TABLE accounts ADD COLUMN budget_carry BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN budget_carry_since DATE;
ALTER TABLE accounts ADD COLUMN need_group TEXT CHECK (need_group IN ('needs', 'wants', 'savings'));
-- isi need_group untuk akun bawaan berdasarkan kode (5101/5102/5105/5107 needs, 5103/5104 wants, 1003/1005 savings)
```
Down: menghapus ketiga kolom. 000025 tidak dipakai.

- **Awal penumpukan:** `budget_carry_since` diisi `CURRENT_DATE` saat sakelar dinyalakan (dan dikosongkan saat dimatikan). Sisa dihitung mulai dari periode yang memuat tanggal itu, jadi riwayat sebelum sakelar menyala tidak pernah ikut. Periode tempat sakelar dinyalakan sudah ikut menyumbang sisa ke periode berikutnya.
- Akun bawaan baru mendapat kelompok dari `model.DefaultNeedGroup` (peta kode → kelompok) saat didaftarkan; tabel template tidak berubah.

## Backend
| Method | Path | Isi |
|---|---|---|
| PATCH | `/accounts/:id` | menerima `budget_carry` (bool) dan `need_group` (`needs`/`wants`/`savings`/`none` untuk mengosongkan) |
| GET | `/budgets` | tiap baris: `budget_carry`, `carry`, `available`, `need_group`; respons: `income`, `groups[{group, amount, share, target}]` |
| GET | `/entries` | dengan `account_id`: tiap entry membawa `balance_after` (saldo akun setelah entry itu, dalam urutan tanggal lalu nomor, tidak terpengaruh halaman atau filter lain) |
| GET | `/entries/export` | `format=csv\|xlsx` (bawaan csv), filter seperti `/entries`; paging diabaikan (maks 50.000 baris); kolom No, Tanggal, Jenis, Keterangan, Debit, Kredit, Jumlah, (Saldo) |

- `Remaining`, `Over`, peringatan anggaran di Insights, dan total anggaran di ringkasan grup memakai `available`; tanpa amplop `available = budget`.
- Kelompok: pengeluaran akun beban dalam kelompok, ditambah transfer neto ke akun aset berkelompok `savings`. Bagian = jumlah ÷ pemasukan periode (0 tanpa pemasukan).
- Periode diambil lewat `periodOf` dan `periodBack`; sisa memakai dua query per periode lalu (maks 120 periode), dibatasi oleh `ponytail:` di `carryOver`.
- Saldo berjalan: satu query window atas seluruh riwayat akun (`RunningBalances`), dicocokkan ke baris halaman; tanda mengikuti sisi normal akun (`signed`).

## Frontend
- `budgets.tsx`: sakelar amplop dan kontrol segmen kelompok per kategori, petunjuk "+ sisa Rp X dari periode lalu" (atau "− kurang …" bila negatif), kartu 50/30/20 dengan penanda target.
- `dashboard.tsx`: bilah anggaran memakai `available` dan menampilkan petunjuk yang sama.
- `transactions.tsx`: tombol Unduh CSV/Excel (memakai `saveFile`, pembantu unduh yang sama dengan ekspor laporan) dan kolom Saldo saat filter akun aktif.
- Kunci i18n baru ada di blok `fase14` di akhir `id.ts` dan `en.ts`.

## Pengujian
`internal/handler/fase14_test.go`: amplop lintas tiga periode (sisa, belanja berlebih mengurangi periode berikutnya, kategori tanpa amplop tetap reset, dashboard sama), bagian 50/30/20 dan pemindahan kelompok, saldo berjalan (urutan, halaman 2, tanpa filter tidak ada), isi CSV (BOM, header, baris, Saldo), XLSX, format tidak dikenal ditolak.

## Di luar cakupan
Anggaran tahunan, proyeksi, dan laporan 12 bulan (ditunda). Ekspor tidak di-stream.
