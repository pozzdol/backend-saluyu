# Fase 6 — Bahasa, Istilah Sehari-hari + ⓘ, Pencatatan Cepat, dan Edit Transaksi

Menjawab masukan B1–B4, B6–B10, A1, A5 (lihat `masukan-dan-fase-lanjutan.md`).

## Tujuan & definisi selesai
1. **Bahasa:** semua teks UI tersedia dalam Indonesia dan English. Pengguna baru dengan browser `id-*` mendapat Indonesia, selain itu English. Pilihan bisa diubah di Profil dan tersimpan di akun.
2. **Pesan error ikut bahasa:** API mengirim `code` stabil (misal `email_taken`, `field.required`), dan frontend menerjemahkannya. Teks `error` dalam English tetap ada sebagai fallback.
3. **Istilah + ⓘ:** setiap judul halaman, label kolom laporan, dan istilah finance di form punya ⓘ. Di desktop terbuka saat hover/fokus, di HP lewat tap yang membuka bottom sheet. Semuanya diambil dari satu kamus per bahasa.
4. **Quick Add:** di HP ada tombol + mengambang di semua halaman `/app`, di desktop tombol "+ Catat" di header. Alurnya: jumlah (keypad) → kategori (ikon, yang sering dipakai di atas) → simpan, dengan tanggal hari ini dan dompet terakhir. Cukup 3 tap untuk pengeluaran biasa.
5. **Kategori berikon & berwarna:** template default sudah punya ikon dan warna, dan akun buatan user bisa memilihnya.
6. **Edit semua jenis transaksi** dengan riwayat perubahan:
   - income/expense/transfer/cash_count/adjustment: edit langsung;
   - opening: lewat halaman saldo awal; boleh walau buku sudah berisi, selama tanggal mulai tidak melewati transaksi pertama;
   - loan/repayment tanpa tautan atau masih pending: edit langsung; permintaan pending ikut diperbarui;
   - loan/repayment yang sudah di-approve: menjadi **permintaan perubahan**; kedua buku berubah bersamaan setelah disetujui.
7. **Batalkan hapus:** setelah hapus, toast "Batalkan" muncul selama 8 detik.
8. Test hijau, plus uji manual ke Postgres sungguhan dan browser (desktop + 390px).

## Di luar cakupan
Lupa password/email (Fase 7), kartu kredit (Fase 8), transaksi berulang (Fase 9), grafik (Fase 10), tutup buku (Fase 11). Terjemahan konten buatan user (nama akun dan kategori) tidak dilakukan: nama akun template tetap bahasa Indonesia.

## Database
**`000013_add_locale_icons_and_revisions.up.sql`**
```sql
-- NULL = follow the browser (id-* gets Indonesian, anything else English).
ALTER TABLE users ADD COLUMN locale VARCHAR(5) CHECK (locale IN ('id', 'en'));

-- Icon is a lucide icon name, color a palette token; both optional (UI falls back by type).
ALTER TABLE accounts ADD COLUMN icon VARCHAR(40), ADD COLUMN color VARCHAR(20);
UPDATE accounts SET icon = t.icon, color = t.color FROM (VALUES
  ('1001','landmark','blue'), ('1002','wallet','green'), ('1003','piggy-bank','teal'), ('1004','smartphone','violet'),
  ('1005','trending-up','indigo'), ('1101','hand-coins','orange'), ('2101','handshake','amber'), ('3001','scale','slate'),
  ('4001','briefcase','green'), ('4002','percent','teal'), ('4003','rotate-ccw','sky'), ('4199','sparkles','yellow'),
  ('5101','car','sky'), ('5102','utensils','orange'), ('5103','cookie','pink'), ('5104','clapperboard','violet'),
  ('5105','house','rose'), ('5107','receipt','slate'), ('5199','coins','zinc')
) AS t(code, icon, color) WHERE accounts.code = t.code;

-- Every edit keeps the entry as it was and as it became.
CREATE TABLE entry_revisions (
  id         BIGSERIAL PRIMARY KEY,
  entry_id   UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  before     JSONB NOT NULL, -- {entry_date, period_date, description, lines:[...]}
  after      JSONB NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX entry_revisions_entry_idx ON entry_revisions (entry_id, changed_at);

-- Editing a loan the other member already mirrored asks them first.
CREATE TYPE change_request_status AS ENUM ('pending', 'approved', 'rejected', 'cancelled');
CREATE TABLE loan_change_requests (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  loan_request_id UUID NOT NULL REFERENCES loan_requests(id) ON DELETE CASCADE,
  requested_by    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  proposed        JSONB NOT NULL, -- the requester's new entry: {entry_date, description, lines}
  status          change_request_status NOT NULL DEFAULT 'pending',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  responded_at    TIMESTAMPTZ
);
-- At most one open change per loan; a new edit replaces (cancels) the pending one.
CREATE UNIQUE INDEX loan_change_requests_one_pending ON loan_change_requests (loan_request_id) WHERE status = 'pending';
```
Down: drop dua tabel dan type, drop kolom icon/color/locale.

Seed register (`model.DefaultAccounts`) ikut menyimpan icon dan color.

## Backend

### Kode error (i18n)
- Setiap sentinel error mendapat kode stabil, lewat satu map di `handler/errors.go`: `ErrEmailTaken → "email_taken"`, `ErrWrongAccountType → "wrong_account_type"`, dan seterusnya.
- `fail()` mengirim `{"error": "...", "code": "..."}`.
- `bindJSON` mengirim `fields` berisi kode seperti `"required"`, `"email"`, `"min:6"`, `"max:100"`, `"oneof"`, `"datetime"`, `"gt:0"`.
- `fields_text` tetap mengirim teks English lama sebagai fallback. Frontend memakai `code` kalau ada di kamus, kalau tidak memakai teks.

### Locale
- `PATCH /me` menerima `locale` (`oneof=id en`, nullable untuk kembali ke "ikuti browser").
- `UserResponse.locale`.

### Akun
- `CreateAccountRequest`/`UpdateAccountRequest` menerima `icon` dan `color` (keduanya divalidasi dengan daftar yang diizinkan).
- `GET /accounts` mengembalikan `icon`, `color`, dan `uses_30d` (jumlah baris jurnal 30 hari terakhir, satu `LEFT JOIN` agregat) untuk urutan "sering dipakai".

### Edit transaksi
Semua jalur edit memakai satu fungsi service, `replace(ctx, userID, entryID, next *model.Entry)`:
1. Muat entry lama (beserta lines) dan permintaan loan terkait.
2. Validasi entry baru lewat `validate()`, `checkContacts()`, dan aturan `start_date` (sama dengan `post()`).
3. Repo `ReplaceEntry` dalam satu transaction:
   1. kunci baris user;
   2. tulis `entry_revisions` (before/after);
   3. update header;
   4. hapus lines lama dan insert lines baru;
   5. hitung ulang `loan_requests.amount` kalau masih pending, dan hapus request kalau kontaknya tidak lagi tertaut.
4. Kalau entry punya `loan_requests` dengan status `approved`, edit langsung ditolak. Service membuat `loan_change_requests` (yang pending sebelumnya dibatalkan) dan mengembalikan `202 {"status":"pending_approval"}`.

| Endpoint | Untuk | Body |
|---|---|---|
| `PUT /entries/:id` | income / expense / transfer | sama dengan `PostEntryRequest` (kind tidak boleh berubah) |
| `PUT /loans/:id` | loan / repayment | sama dengan `LoanRequest`; pengecekan kelebihan bayar mengabaikan baris entry itu sendiri |
| `PUT /entries/:id/lines` | cash_count / adjustment (dan koreksi lanjutan) | `{entry_date, description, lines:[{account_id, contact_id?, debit, credit}]}` |
| `PUT /opening` | opening | seperti sekarang; aturan "buku kosong" diganti dengan "start_date ≤ tanggal transaksi pertama" |
| `GET /entries/:id/revisions` | riwayat | `{data:[{changed_at, before, after}]}` |

### Permintaan perubahan talangan
- `GET /inbox` menambah `changes: [{id, from_name, before:{amount, date}, after:{amount, date}, created_at}]`.
- `POST /loan-change-requests/:id/approve` berjalan dalam satu transaction:
  1. kunci request (`FOR UPDATE`; idempoten);
  2. terapkan `proposed` ke entry pengirim (lewat `ReplaceEntry`, dengan revisi);
  3. bangun ulang entry cerminan penerima dengan `mirrorLines()`, memakai akun "sisi saya" yang sama seperti sebelumnya (dengan revisi);
  4. update `loan_requests.amount`;
  5. tandai approved.
- `POST /loan-change-requests/:id/reject` → 204.
- Pengirim melihat status "Menunggu persetujuan" di entry-nya.

### Error baru
- `ErrKindChange` (400): mengubah jenis transaksi tidak diizinkan; buat transaksi baru.
- `ErrStartAfterEntries` (409): tanggal mulai buku melewati transaksi pertama.
- `ErrChangePending` (409): kalau perlu.

## Frontend

### i18n (`src/lib/i18n.ts`)
- Kamus `id` dan `en` sebagai objek TypeScript (kunci bertingkat, misalnya `nav.dashboard` atau `tx.save`). Tipe kunci diambil dari kamus `id`, jadi kunci yang salah ketik gagal di `tsc`.
- `useT()` mengembalikan `t(key, vars)` dan mengikuti `user.locale ?? browser`.
- `fmtDate`/`fmtIDR` mengikuti locale (contoh: `16 Okt 2026` dan `16 Oct 2026`).
- `apiErrorText(e)` menerjemahkan `code` atau `fields`.
- Ponytail: tanpa library i18n. Interpolasi `{name}` cukup dengan satu `replace`.

### ⓘ (`src/components/term.tsx` + `src/lib/glossary.ts`)
- Kamus berisi kunci, label sehari-hari, istilah resmi, penjelasan satu-dua kalimat, dan contoh angka opsional, per bahasa. Isi awalnya sekitar 25 istilah: aset, kewajiban, ekuitas/kekayaan bersih, pendapatan, beban, debit, kredit, saldo normal, jurnal, buku besar, neraca saldo, laba rugi, neraca, saldo awal, rekonsiliasi, hitung kas, piutang, utang, talangan, periode gajian, budget diwarisi, saving rate, surplus, laba berjalan, dan jurnal penyesuaian.
- `<Term k="trialBalance" />` menampilkan label beserta tombol ⓘ (`aria-label` "Apa itu …"):
  - desktop: Popover yang terbuka saat hover/fokus dan tertutup saat keluar;
  - HP: tap membuka Sheet bawah.
- `PageTitle` di AppShell menerima `term` sehingga setiap judul halaman punya ⓘ.

### Quick Add (`src/components/quick-add.tsx`)
- Di HP berupa FAB bulat (56px) di kanan bawah dengan jarak aman dari home bar, muncul di semua halaman `/app` kecuali saat sheet lain terbuka. Di desktop berupa tombol di header AppShell.
- Isi sheet:
  - segmented Keluar / Masuk / Pindah;
  - **Keypad** (`src/components/keypad.tsx`): tombol 1–9, 000, 0, ⌫, dengan tampilan "Rp 25.000" yang langsung terformat;
  - **grid kategori berikon** (4 kolom, diurutkan dari `uses_30d`);
  - chip dompet (terakhir dipakai, disimpan di `localStorage` per user) dan chip tanggal (hari ini, pakai `DatePicker`);
  - catatan opsional;
  - tombol Simpan.
- Setelah tersimpan, muncul toast "Tersimpan · Batalkan" (batalkan = hapus entry yang baru dibuat).

### Ikon & warna
- `src/lib/icons.ts` memetakan nama ikon yang diizinkan ke komponen lucide, plus palet warna.
- `AccountIcon` dipakai di picker, daftar jurnal, dashboard, dan Accounts.
- Form tambah/edit akun punya picker ikon (grid) dan picker warna.

### Edit
- Baris jurnal (tabel dan kartu) bisa diklik untuk membuka **Detail transaksi** dalam sheet (HP) atau dialog (desktop). Isinya tanggal, jenis, baris debit/kredit dengan ⓘ, riwayat perubahan, status "Menunggu persetujuan", dan tombol Edit/Hapus.
- Edit membuka form yang sesuai dengan jenisnya, sudah terisi:
  - form transaksi biasa, form pinjaman, editor baris (untuk cash_count/adjustment), atau halaman saldo awal;
  - semuanya memakai komponen yang sudah ada, dengan mode `edit`.
- **Batalkan hapus** dikerjakan di sisi klien: baris langsung disembunyikan, toast 8 detik muncul, dan `DELETE` baru dikirim setelah waktu habis atau saat halaman ditinggalkan (`pagehide` memakai `fetch keepalive`).
  - Ponytail: tanpa soft-delete di DB, karena soft-delete menuntut filter `deleted_at` di semua query laporan. Kalau tab ditutup sebelum 8 detik, hapusnya tetap terkirim lewat keepalive.
- Inbox Groups menambah kartu "Perubahan talangan" (sebelum → sesudah) dengan tombol Setujui/Tolak.

### Toast (`src/components/toast.tsx`)
Komponen toast global (satu antrean): `aria-live="polite"`, posisi di atas FAB, dan bisa ditutup.

## Test
- **Go (handler, dengan fake):**
  - `TestEditSimpleEntry`: edit jumlah dan kategori pengeluaran, lalu trial balance tetap seimbang, ledger lama dan baru berubah, dan ada 1 revisi dengan before/after yang benar. Mengubah kind ditolak dengan 400.
  - `TestEditLoan`: edit talangan yang belum ditautkan memperbarui piutang. Edit pelunasan memakai pengecekan kelebihan bayar tanpa menghitung dirinya sendiri.
  - `TestEditApprovedSharedLoan`: edit menghasilkan 202 pending dan kedua buku belum berubah. Setelah approve (dua kali, hasil sama), kedua buku berubah dan seimbang, masing-masing dengan 1 revisi. Kalau ditolak, tidak ada yang berubah.
  - `TestOpeningEditable`: saldo awal bisa diubah walau ada transaksi; tanggal mulai setelah transaksi pertama ditolak dengan 409.
  - `TestErrorCodes`: `email_taken`, `field required`, dan `wrong_account_type` muncul di respons.
  - `TestAccountsUsage`: `uses_30d` menghitung baris 30 hari saja.
- **Frontend:** `node --experimental-strip-types` untuk `i18n.test.ts` (setiap kunci `id` ada di `en` dan sebaliknya, interpolasi benar) dan `keypad.test.ts` (urutan tombol menghasilkan sen yang benar, termasuk 000 dan ⌫).
- **Manual:**
  - smoke test ke Postgres (edit, revisi, alur permintaan perubahan dengan 2 user, dan approve paralel 10× tetap 1 perubahan);
  - browser: id/en, ⓘ dengan hover/keyboard/tap di HP, Quick Add 3 tap di 390px, undo hapus, probe pergeseran layout.

## Langkah kerja berurutan
1. Migration 000013 lalu `migrate up`/`down`/`up`.
2. Backend kode error + locale, lalu test kode error.
3. Akun: icon/color/uses_30d, lalu seed template.
4. `ReplaceEntry` + revisi + endpoint edit (simple, loan, lines, opening), lalu test.
5. Permintaan perubahan talangan + inbox, lalu test.
6. Smoke test ke Postgres.
7. Frontend: i18n infra + kamus id/en, lalu ganti semua teks di halaman yang ada (dikerjakan per halaman, `tsc` di setiap langkah).
8. Glossary + `Term` + ⓘ di judul halaman dan istilah.
9. Ikon & warna akun, `AccountIcon`, picker ikon.
10. Keypad, toast, Quick Add.
11. Detail transaksi + edit per jenis + undo hapus + kartu permintaan perubahan di inbox.
12. Probe browser (id/en, 1280/390, pergeseran layout, pointer), lalu commit.

## Risiko & catatan
- **Jumlah teks yang diterjemahkan besar** (semua halaman). Kunci bertipe mencegah kunci hilang, dan test kamus memastikan kedua bahasa lengkap.
- **Edit opening yang mengubah `start_date`** setelah ada transaksi: dibatasi ke "≤ tanggal transaksi pertama", jadi tidak ada transaksi yang berada sebelum tanggal mulai.
- **Permintaan perubahan butuh akun "sisi saya" penerima** dari mirror lama; diambil dari baris non-kontak entry cerminan.
- **Undo di sisi klien** bisa gagal kalau browser crash sebelum 8 detik. Akibatnya transaksi tidak terhapus, yang merupakan arah yang aman.
