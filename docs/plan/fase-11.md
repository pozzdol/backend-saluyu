# Fase 11 — Tujuan, Tutup Buku, PWA, dan Berbagi

Menjawab sebagian A2, A3, A6, A20, A21, A23–A25, A28, B16, B17, B19, B20 (lihat `masukan-dan-fase-lanjutan.md`).

Fase 11 di rencana awal terlalu besar untuk satu langkah, jadi dibagi dua:

- **11a (fase ini):** tujuan/sinking fund, tutup buku + jurnal manual + jurnal pembalik, PWA dengan antrean offline Quick Add, berbagi lewat WhatsApp (tagih utang, link undangan grup).
- **11b (dipindah):** isinya sudah dibagi ke rencana putaran 2 di [masukan-putaran-2.md](masukan-putaran-2.md): mode gelap → Fase 12, anggaran amplop → Fase 14, investasi dan multi-mata uang → Fase 16, lampiran dan tag → Fase 17. Rincian semula: mode gelap (perlu menyapu ±80 warna terang yang ditulis langsung di kelas), lampiran struk (butuh tempat simpan file: disk server atau penyimpanan objek seperti S3/R2), tag, investasi dengan nilai pasar, multi-mata uang, anggaran amplop dengan sisa terbawa.

## Definisi selesai (11a)
1. **Tujuan:** nama, target, tenggat (opsional), akun penampung. Progres = saldo akun penampung; "perlu disisihkan per bulan" = sisa ÷ bulan tersisa. Tampil di Ringkasan.
2. **Tutup buku:** pengguna mengunci buku sampai suatu tanggal (akhir periode). Mencatat, mengubah, menghapus, atau menyetujui transaksi bertanggal ≤ tanggal kunci ditolak dengan pesan jelas (409 `period_locked`, menyebut tanggalnya). Buka kunci tetap bisa (mundurkan tanggal).
3. **Jurnal manual** multi-baris (menu Lanjutan) dan tombol **"Balik"** di detail transaksi: membuat jurnal pembalik bertanggal hari ini, untuk mengoreksi periode yang terkunci.
4. **PWA:** bisa dipasang di layar HP (manifest + ikon 192/512 + service worker). App shell tetap terbuka saat offline.
5. **Quick Add offline:** saat offline atau jaringan gagal, catatan masuk antrean di perangkat dan dikirim saat online lagi. Setiap catatan membawa `client_id`; server menolak dobel (indeks unik per user), jadi kirim ulang tidak membuat transaksi ganda.
6. **WhatsApp:** di halaman kontak, tombol "Tagih lewat WhatsApp" (pesan berisi jumlah utang). Di grup, "Undang lewat link": link bertoken sekali pakai, berlaku 7 hari, dibagikan lewat WhatsApp atau disalin.

## Database
**`000017_add_goals_locks_and_links.up.sql`**
```sql
CREATE TABLE goals (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       VARCHAR(100) NOT NULL,
  target     BIGINT NOT NULL CHECK (target > 0),
  deadline   DATE,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, -- the savings that hold it
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Entries dated on or before this day can no longer change.
ALTER TABLE users ADD COLUMN locked_until DATE;

-- Set by clients that may retry (the offline Quick Add queue): the same id posts once.
ALTER TABLE journal_entries ADD COLUMN client_id UUID;
CREATE UNIQUE INDEX journal_entries_client_idx ON journal_entries (user_id, client_id) WHERE client_id IS NOT NULL;

CREATE TABLE group_invite_links (
  token_hash BYTEA PRIMARY KEY,
  group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  used_by    UUID REFERENCES users(id) ON DELETE SET NULL,
  used_at    TIMESTAMPTZ
);
```

## Backend
| Method | Path | Isi |
|---|---|---|
| GET/POST | `/goals` | daftar (dengan saldo, sisa, per bulan) / buat |
| PUT/DELETE | `/goals/:id` | ganti / hapus |
| PUT | `/me/lock` | `{locked_until: "YYYY-MM-DD" \| null}` |
| POST | `/entries/journal` | `{entry_date, description, lines[]}` → jurnal manual (`adjustment`) |
| POST | `/entries/:id/reverse` | `{entry_date?}` → jurnal pembalik (`adjustment`, baris dibalik) |
| POST | `/entries` | menerima `client_id` (opsional); id yang sama → entry yang sudah ada, 200 |
| POST | `/groups/:id/invite-links` | → `{url, expires_at}` |
| POST | `/invite-links/accept` | `{token}` → bergabung ke grup |

- **Kunci** diperiksa di satu tempat: `LedgerService.prepare` (semua jalur posting: form, kartu, cicilan, berulang, impor) ditambah edit (tanggal lama dan baru), hapus, saldo awal, dan persetujuan talangan. `UserResponse.locked_until` ikut dikirim.
- **Link undangan** membuktikan kepemilikan lewat token, jadi tidak perlu email yang cocok atau terverifikasi.

## Frontend
- **Tujuan:** bagian di Ringkasan + kelola di halaman Anggaran (form nama, target, tenggat, akun).
- **Tutup buku:** di halaman Laporan, "Tutup buku sampai akhir periode ini" / "Buka kunci". Tanggal kunci tampil di jurnal.
- **Jurnal manual:** halaman `/app/journal` (menu "Jurnal Manual" dengan ⓘ), memakai editor baris yang sudah ada. Detail transaksi: tombol "Balik".
- **PWA:** `public/manifest.webmanifest`, ikon dari `public/apple-touch-icon.png` (ukuran 192/512 dibuat ulang), `public/sw.js` (cache app shell dan aset, API tidak pernah di-cache), pendaftaran di `main.tsx` hanya di build produksi.
- **Antrean offline:** `lib/outbox.ts` (localStorage), dipakai Quick Add; badge "n menunggu sinkron"; sinkron saat `online` dan saat aplikasi dibuka.
- **WhatsApp:** `https://wa.me/?text=…` (pengguna memilih penerima di WhatsApp; nomor tidak disimpan di app).

## Test
- Kunci: posting, edit (tanggal lama/baru), hapus, saldo awal, impor ke tanggal terkunci ditolak; jurnal pembalik di tanggal terbuka diterima.
- `client_id` dikirim dua kali (juga paralel di Postgres) → satu entry.
- Link undangan: sekali pakai, kedaluwarsa, pemilik grup tidak bisa memakai link sendiri.
- Tujuan: per bulan = sisa ÷ bulan tersisa.
