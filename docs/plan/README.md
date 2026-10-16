# Rencana digitalisasi Excel "Financial Analyst - Lida"

Lima fase. Setiap fase bisa dipakai sendiri dan bergantung hanya pada fase sebelumnya.

| Fase | Isi | Migration | Dokumen |
|---|---|---|---|
| 1 | Buku besar inti: COA per user, form Pemasukan/Pengeluaran/Transfer → jurnal otomatis, buku besar, neraca saldo | 000004–000005 | [fase-1.md](fase-1.md) |
| 2 | Saldo awal, hitung kas, rekonsiliasi | 000006 | [fase-2.md](fase-2.md) |
| 3 | Periode gajian, budget, dashboard, Laba Rugi, Neraca | 000007–000008 | [fase-3.md](fase-3.md) |
| 4 | Kontak, talangan, split bill, pelunasan | 000009–000010 | [fase-4.md](fase-4.md) |
| 5 | Grup berbagi, loan request lintas buku dengan approve | 000011–000012 | [fase-5.md](fase-5.md) |

## Keputusan produk (sudah disepakati)
- Buku per user. Grup berbagi (2+ orang) aktif hanya jika semua anggota menyetujui. Anggota hanya melihat ringkasan, bukan detail transaksi.
- Input lewat form sederhana; sistem membuat jurnal double-entry.
- Periode = siklus gajian, tanggal mulai per user (default 25).
- Talangan = piutang per kontak. Jika kontak adalah anggota grup, buku dia mencatat utang otomatis setelah approve.
- Mulai baru dengan saldo awal riil, tanpa impor histori Excel.
- Kas tunai lewat hitung kas berkala; selisih ke 5199 Beban Tunai Lain-lain.
- Uang disimpan `BIGINT` dalam sen.

## Kontrak lintas fase (hasil penyelarasan)
Kelima rencana disusun paralel, lalu diselaraskan. Aturan yang mengikat semua fase:

1. **Satu titik posting.** `LedgerService.post(ctx, userID, entry, lines)` (Fase 1) adalah satu-satunya jalan menulis jurnal: cek kepemilikan akun, Σdebit = Σcredit, guard `start_date` (Fase 2), aturan `contact_id` untuk 1101/2101 (Fase 4), pembuatan `loan_requests` (Fase 5). Form Fase 1 hanya pembungkus yang membangun lines.
2. **Kode akun template:** 1101 Piutang, 2101 Utang, 3001 Ekuitas Saldo Awal, 5199 Beban Tunai Lain-lain. Rencana Fase 5 semula menyebut 1201; sudah dikoreksi ke 1101.
3. **Kategori akun:** Tabungan memakai category `Tabungan`, Investasi memakai `Investasi`. Dashboard Fase 3 menghitung "transfer ke tabungan/investasi" dari dua category ini.
4. **Nomor migration** diurutkan ulang seperti tabel di atas (rencana asli saling tumpang tindih di 000008–000009).
5. **Query tanggal** selalu `?date=YYYY-MM-DD` dari browser (tanggal lokal), termasuk ringkasan grup Fase 5. Server tidak menebak "hari ini" dalam UTC.
6. **Frontend bersama:** `fmtIDR`/`toSen` di `src/lib/utils.ts`; `Protected` menerima komponen (dibuat di Fase 1); `useForm`/`Status` dipindah ke `field.tsx` (Fase 2); semua halaman aplikasi di bawah `/app/*`.
7. **Hapus entry:** `opening` hanya lewat `PUT /opening` (Fase 2); entry dengan loan request approved terkunci (Fase 5).

## Keputusan lanjutan (putaran diskusi kedua)
| Topik | Keputusan | Berdampak ke |
|---|---|---|
| Refund belanja | Dicatat sebagai Pemasukan ke **4003 Pengembalian**. Form tidak pernah mengkredit akun beban, jadi saldo beban tidak bisa negatif | Fase 1 |
| Kas lebih saat hitung kas | Kredit **4199 Pendapatan Lain-lain**; kas kurang tetap debit 5199 | Fase 2 |
| Gaji masuk lebih awal | Checkbox "untuk periode berikutnya" di form Pemasukan → kolom `journal_entries.period_date`. Laba Rugi/dashboard memakai `COALESCE(period_date, entry_date)`, Neraca tetap `entry_date` | Fase 3 |
| Seed COA | Kode Go saat register, satu template "Pribadi" (19 akun), dalam transaction yang sama dengan insert user. Migration 000004 hanya men-seed user lama | Fase 1 |
| Undangan grup | Pengundang melihat daftar email yang menunggu. Undangan disimpan per email di tabel `group_invites`, termasuk email yang belum terdaftar, supaya daftar itu tidak membocorkan siapa yang punya akun | Fase 5 |
| Investasi | Harga beli saja (transfer ke 1005 Investasi), seperti Excel sekarang. Tanpa nilai pasar dan tanpa rekomendasi | Fase 1 |

Template COA final: 1001 Bank, 1002 Cash, 1003 Tabungan, 1004 E-wallet, 1005 Investasi, 1101 Piutang, 2101 Utang, 3001 Ekuitas Saldo Awal, 4001 Gaji, 4002 Bunga, 4003 Pengembalian, 4199 Pendapatan Lain-lain, 5101 Transportasi, 5102 Makan, 5103 Jajan, 5104 Entertainment, 5105 Keluarga, 5107 Admin, 5199 Beban Tunai Lain-lain.
