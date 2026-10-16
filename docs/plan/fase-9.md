# Fase 9 — Transaksi Berulang, Pengingat, Impor Mutasi, dan Aturan Kategori

Menjawab masukan A10–A13 dan B18 (lihat `masukan-dan-fase-lanjutan.md`).

## Tujuan & definisi selesai
1. **Transaksi berulang:** gaji, sewa, langganan, zakat. Pola harian, mingguan, bulanan (tanggal tertentu), atau tiap gajian. Mode **pengingat** (default: muncul di "Siap dicatat", tinggal konfirmasi atau lewati) atau **otomatis** (diposting sendiri).
2. **Idempoten:** gaji tanggal 25 dengan mode otomatis terposting tepat sekali, walau job berjalan dua kali atau bersamaan.
3. **Impor mutasi CSV/XLSX:** unggah file, kolom terdeteksi otomatis dari judulnya (bisa diubah, tersimpan per akun), pratinjau, lalu posting semua dalam satu transaksi DB. Impor ulang file yang sama tidak membuat duplikat.
4. **Rekonsiliasi per transaksi:** baris mutasi yang cocok dengan transaksi yang sudah dicatat manual (akun sama, jumlah sama, tanggal ±3 hari) ditandai "sudah tercatat" dan ditautkan, bukan dibuat ulang.
5. **Aturan kategori:** kata kunci (dan rentang jumlah opsional) → kategori, misalnya "GRAB" → Transportasi. Dipakai saat impor dan di Quick Add (saran kategori dari catatan).
6. Test hijau, uji Postgres (impor 200 baris dua kali, job ganda), browser 390px.

## Di luar cakupan
Preset kolom per bank yang ditulis tangan (format ekspor tiap bank berubah-ubah; deteksi judul kolom + pemetaan tersimpan per akun lebih tahan), PDF e-statement, pengingat lewat email/push (Fase 11 bersama PWA), jumlah berulang yang berubah-ubah (dipakai jumlah terakhir; bisa diubah saat konfirmasi).

## Database
**`000016_add_recurring_and_imports.up.sql`**
```sql
CREATE TYPE recurring_frequency AS ENUM ('daily', 'weekly', 'monthly', 'payday');
CREATE TYPE recurring_mode AS ENUM ('remind', 'auto');

CREATE TABLE recurring_rules (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            VARCHAR(100) NOT NULL,
  kind            VARCHAR(10) NOT NULL CHECK (kind IN ('income', 'expense', 'transfer')),
  from_account_id UUID NOT NULL REFERENCES accounts(id),
  to_account_id   UUID NOT NULL REFERENCES accounts(id),
  amount          BIGINT NOT NULL CHECK (amount > 0),
  frequency       recurring_frequency NOT NULL,
  day             SMALLINT,              -- weekly: 0 (Sun)..6, monthly: 1..31 (clamped to month end)
  mode            recurring_mode NOT NULL DEFAULT 'remind',
  next_date       DATE NOT NULL,         -- the first occurrence not handled yet
  end_date        DATE,
  last_error      TEXT,                  -- why the last auto-post failed (it then waits as a reminder)
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX recurring_rules_due_idx ON recurring_rules (next_date);

-- One row per handled occurrence: the primary key is what makes posting idempotent.
CREATE TABLE recurring_occurrences (
  rule_id  UUID NOT NULL REFERENCES recurring_rules(id) ON DELETE CASCADE,
  due_date DATE NOT NULL,
  entry_id UUID REFERENCES journal_entries(id) ON DELETE SET NULL, -- NULL = skipped
  PRIMARY KEY (rule_id, due_date)
);

CREATE TABLE category_rules (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  keyword    VARCHAR(100) NOT NULL,       -- matched case-insensitively inside the description
  min_amount BIGINT,                      -- sen, optional range
  max_amount BIGINT,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX category_rules_user_idx ON category_rules (user_id);

-- Column mapping remembered per account (the bank or e-wallet the file came from).
CREATE TABLE import_profiles (
  account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  mapping    JSONB NOT NULL
);

-- Every imported (or matched) statement row, by hash, so re-importing finds it.
CREATE TABLE imported_rows (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  row_hash   BYTEA NOT NULL,
  entry_id   UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
  PRIMARY KEY (account_id, row_hash)
);
CREATE INDEX imported_rows_entry_idx ON imported_rows (entry_id);
```

## Backend
### Berulang
| Method | Path | Isi |
|---|---|---|
| GET/POST | `/recurring` | daftar / buat aturan (`name, kind, from_account_id, to_account_id, amount, frequency, day, mode, start_date, end_date`) |
| PUT/DELETE | `/recurring/:id` | ganti / hapus |
| GET | `/recurring/due` | kejadian yang menunggu (≤ hari ini), setelah aturan otomatis milik user diproses |
| POST | `/recurring/:id/confirm` | `{due_date, amount?, entry_date?}` → posting kejadian itu |
| POST | `/recurring/:id/skip` | `{due_date}` |

- `Occurrences(rule, until)`: tanggal-tanggal dari `next_date` sampai `until`. Bulanan tanggal 31 → akhir bulan; gajian = `period_start_day` user.
- **Job:** goroutine di `main` menjalankan `RunAuto` saat start lalu tiap jam: untuk setiap aturan `auto` yang `next_date ≤ hari ini`, posting tiap kejadian dalam transaksi DB yang menyisipkan `recurring_occurrences` dulu (`ON CONFLICT DO NOTHING`; 0 baris = sudah ditangani pihak lain, rollback). Gagal validasi → `last_error` diisi, kejadian tetap menunggu sebagai pengingat.
- `next_date` dimajukan melewati kejadian yang sudah ditangani.

### Impor
| Method | Path | Isi |
|---|---|---|
| POST | `/imports/preview` | multipart: `file` (CSV/XLSX, ≤5 MB), `account_id`, `mapping` (opsional, JSON) → `{mapping, rows:[{line, date, description, amount, balance, hash, status: new\|duplicate\|matched, entry_id?, suggested_account_id?}], errors:[{line, message}]}` |
| POST | `/imports/commit` | `{account_id, mapping, rows:[{hash, date, description, amount, account_id?, match_entry_id?}]}` → posting baris baru (pemasukan/pengeluaran sesuai tanda) + menautkan yang cocok, satu transaksi DB; simpan pemetaan |
| GET/POST/DELETE | `/category-rules` | aturan kategori |

- **Pemetaan:** `{header_row, date_col, description_cols[], amount_col | debit_col+credit_col, date_format (dmy\|mdy\|ymd), decimal (comma\|dot)}`. Deteksi otomatis mencari baris judul dengan kata seperti tanggal/date, keterangan/description/remark, debit/debet/keluar, kredit/credit/masuk, jumlah/amount/nominal/mutasi, saldo/balance. Jumlah boleh berakhiran `DB`/`CR`, bertanda minus, atau dalam kurung.
- **XLSX** dibaca dengan stdlib (`archive/zip` + `encoding/xml`): sheet pertama, shared strings, tanggal serial Excel.
- **Hash** = SHA-256(tanggal | jumlah | keterangan dinormalkan | urutan kemunculan baris identik di file). Dua kopi Rp10.000 di hari yang sama tetap dua baris.
- Baris positif = uang masuk (Dr akun ini / Cr kategori pemasukan, default 4199), negatif = keluar (Dr kategori pengeluaran, default 5199 / Cr akun ini). Untuk kartu kredit tandanya dibalik oleh pengguna lewat pemetaan (`negate`).

## Frontend
- **Berulang** (`/app/recurring`, menu "Berulang"): daftar aturan + form; Ringkasan menampilkan kartu **"Siap dicatat"** dengan Catat / Lewati.
- **Impor** (`/app/import`, menu "Impor Mutasi"): pilih akun + file → pemetaan (terdeteksi, bisa diubah) → pratinjau (status per baris, kategori per baris yang terisi dari aturan, centang "ingat: kata kunci → kategori") → Impor. Bagian "Aturan kategori".
- **Quick Add:** saat catatan diketik, kategori yang cocok dengan aturan dipilih otomatis (kalau belum dipilih).

## Test
- `recurring/occurrences_test.go`: harian, mingguan, bulanan 31 → Feb, gajian, end_date.
- `statement/parse_test.go`: CSV ber-BOM dengan `;`, kolom debit/kredit terpisah, `DB/CR`, angka `1.234.567,89` dan `1,234,567.89`, XLSX kecil buatan test, deteksi judul.
- Handler: konfirmasi/lewati, auto-post dua kali = satu entry, impor → impor ulang = semua duplikat, baris cocok ditautkan.

## Langkah kerja
1. Migration. 2. Paket `recurring` + test. 3. Service/repo berulang + job. 4. Paket `statement` (CSV/XLSX/deteksi) + test. 5. Service/repo impor + aturan kategori. 6. Test handler + uji Postgres. 7. Frontend. 8. Browser.
