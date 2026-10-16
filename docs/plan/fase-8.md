# Fase 8 — Kartu Kredit, Paylater, Cicilan, dan Transfer Berbiaya

Menjawab masukan A4, A7, A8, A9 (lihat `masukan-dan-fase-lanjutan.md`).

## Tujuan & definisi selesai
1. **Kartu kredit dan paylater sebagai sumber pembayaran:** belanja memakai kartu dicatat Dr Beban / Cr Utang Kartu, jadi pengeluaran muncul di periode belanja, bukan saat tagihan dibayar.
2. **Tagihan kartu:** tanggal cetak, jatuh tempo, limit, minimum pembayaran. Halaman kartu menampilkan tagihan terakhir (total, minimum, sudah dibayar, sisa, status) dan transaksi berjalan. Tombol "Bayar tagihan" adalah transfer Bank → Kartu. Ringkasan memberi peringatan tagihan yang jatuh tempo dalam 7 hari.
3. **Cicilan berbunga** (KPR, kredit motor, KTA, paylater bertenor): jadwal angsuran flat atau anuitas yang memisahkan pokok dan bunga. Membayar satu angsuran = Dr Utang (pokok) + Dr Beban Bunga / Cr Bank.
4. **Transfer dengan biaya** dalam satu entry: Dr Tujuan + Dr Beban Admin / Cr Sumber.
5. Utang kartu dan cicilan tampil di neraca seperti utang lain (tidak perlu perubahan laporan).
6. Test hijau: tabel jadwal anuitas/flat, siklus tagihan, aturan entry baru; uji Postgres; browser 390px.

## Di luar cakupan
Cicilan 0% di dalam tagihan kartu, bunga kartu otomatis dan denda keterlambatan otomatis (dicatat manual sebagai beban), suku bunga mengambang, pelunasan dipercepat (catat sebagai transfer ke akun utang), tarik tunai kartu.

## Database
**`000015_add_cards_and_installments.up.sql`**
```sql
ALTER TYPE entry_kind ADD VALUE IF NOT EXISTS 'borrowing';   -- loan money received (or a purchase on credit)
ALTER TYPE entry_kind ADD VALUE IF NOT EXISTS 'installment'; -- one scheduled payment: principal + interest

-- general = everything so far; the other kinds are liabilities with extra behaviour.
ALTER TABLE accounts ADD COLUMN kind VARCHAR(20) NOT NULL DEFAULT 'general'
  CHECK (kind IN ('general', 'credit_card', 'paylater', 'loan'));

CREATE TABLE cards (
  account_id      UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  statement_day   SMALLINT NOT NULL CHECK (statement_day BETWEEN 1 AND 28),
  due_day         SMALLINT NOT NULL CHECK (due_day BETWEEN 1 AND 28),
  credit_limit    BIGINT NOT NULL DEFAULT 0 CHECK (credit_limit >= 0), -- sen, 0 = not set
  min_payment_pct SMALLINT NOT NULL DEFAULT 10 CHECK (min_payment_pct BETWEEN 0 AND 100)
);

CREATE TYPE installment_method AS ENUM ('flat', 'annuity');
CREATE TABLE installment_plans (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id      UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE, -- the loan (liability)
  interest_account_id UUID NOT NULL REFERENCES accounts(id),                      -- an expense
  principal       BIGINT NOT NULL CHECK (principal > 0),        -- sen, the original loan
  annual_rate_bp  INT NOT NULL CHECK (annual_rate_bp BETWEEN 0 AND 10000), -- 1050 = 10.50% a year
  tenor_months    INT NOT NULL CHECK (tenor_months BETWEEN 1 AND 360),
  method          installment_method NOT NULL,
  first_due_date  DATE NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A paid installment points at its journal entry; deleting the entry makes it unpaid again.
CREATE TABLE installment_payments (
  plan_id  UUID NOT NULL REFERENCES installment_plans(id) ON DELETE CASCADE,
  seq      INT NOT NULL,
  entry_id UUID NOT NULL UNIQUE REFERENCES journal_entries(id) ON DELETE CASCADE,
  PRIMARY KEY (plan_id, seq)
);

-- New default expense for interest and late fees; existing users get it unless 5108 is taken.
INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color)
SELECT id, '5108', 'Bunga & Denda', 'Expense', 'Beban', 'Debit', 'percent', 'red' FROM users
ON CONFLICT (user_id, code) DO NOTHING;
```

## Aturan entry
| Form | Dari (Cr) | Ke (Dr) | Catatan |
|---|---|---|---|
| expense | Asset **atau Liability** | Expense | belanja pakai kartu: Cr Utang Kartu |
| income | Revenue | Asset | tetap |
| transfer | Asset **atau Liability** | Asset **atau Liability** | bayar tagihan: Bank → Kartu. Boleh `fee` + `fee_account_id` (Expense, default 5107 Admin) → Dr Tujuan (jumlah) + Dr Biaya (fee) / Cr Sumber (jumlah + fee) |
| borrowing | akun cicilan (Liability) | Asset atau Expense | dibuat otomatis saat cicilan baru dengan "uang masuk ke" |
| installment | Asset | akun cicilan + akun bunga | lewat `POST /installments/:id/pay` |

Akun `1101`/`2101` tetap hanya lewat form kontak. `borrowing` dan `installment` tidak bisa diedit (hapus lalu catat ulang), supaya jadwal tetap konsisten.

## Jadwal angsuran (`internal/installment`)
- **Flat:** pokok per bulan = P / n, bunga per bulan = P × r / 12 (tetap).
- **Anuitas:** angsuran A = P·i / (1 − (1+i)^−n), i = r/12. Bunga bulan k = sisa pokok × i, pokok = A − bunga.
- Dihitung dalam sen dengan pembulatan ke sen terdekat setiap baris. Selisih pembulatan masuk ke angsuran terakhir, sehingga total pokok tepat P.
- Jatuh tempo bulan k = `first_due_date` + (k−1) bulan, dengan tanggal dipotong ke akhir bulan bila perlu (31 Jan → 28/29 Feb).

## Siklus tagihan kartu
- Tanggal cetak S. Tagihan terakhir menutup tanggal cetak terbaru yang ≤ hari ini, dan mencakup (cetak sebelumnya, cetak ini].
- Jatuh tempo = tanggal `due_day` pertama setelah tanggal cetak.
- **Total tagihan** = saldo utang kartu pada tanggal cetak (termasuk sisa tagihan sebelumnya).
- **Minimum** = min(total, max(pct% × total, Rp50.000)).
- **Sudah dibayar** = debit ke kartu setelah tanggal cetak; **sisa** = max(0, total − dibayar).
- **Status:** `paid` (sisa 0), `due` (sebelum/tepat jatuh tempo), `overdue` (lewat jatuh tempo dan minimum belum terbayar).

## Backend
| Method | Path | Isi |
|---|---|---|
| POST | `/cards` | `{name, kind: credit_card\|paylater, statement_day, due_day, credit_limit, min_payment_pct}` → akun Liability baru (kode berikutnya dari 2201) + pengaturan kartu |
| PATCH | `/cards/:id` | ubah pengaturan kartu |
| GET | `/cards` | semua kartu dengan ringkasan tagihan terakhir (untuk Ringkasan dan daftar) |
| GET | `/cards/:id` | kartu + tagihan terakhir + transaksi berjalan |
| POST | `/installments` | `{name, principal, annual_rate, tenor_months, method, first_due_date, interest_account_id?, received_into_id?, entry_date?}` → akun cicilan (kode dari 2301), rencana, dan bila `received_into_id` ada, entry `borrowing` |
| GET | `/installments` | daftar rencana dengan angsuran berikutnya, sisa pokok, jumlah terbayar |
| GET | `/installments/:id` | rencana + jadwal lengkap (status per baris: dibayar dengan entry_no, atau belum) |
| POST | `/installments/:id/pay` | `{from_account_id, entry_date}` → membayar angsuran belum lunas yang paling awal |
| DELETE | `/installments/:id` | hanya bila belum ada pembayaran; akun cicilan dinonaktifkan |

`Account` mendapat `kind`. Error baru: `not_a_card` (404), `already_paid` (409, semua angsuran lunas), `plan_has_payments` (409), `wrong_fee` (400, fee di luar transfer atau akun biaya bukan Expense).

## Frontend
- **Kartu** (`/app/cards`, menu "Kartu & Cicilan"): daftar kartu (sisa tagihan, jatuh tempo, sisa limit) dan cicilan (angsuran berikutnya). Form tambah kartu.
- **Detail kartu** (`/app/cards/:id`): tagihan terakhir (total, minimum, sisa, jatuh tempo, status), tombol "Bayar tagihan" (sheet: dari akun, jumlah dengan pilihan sisa/minimum), transaksi berjalan, pengaturan.
- **Detail cicilan** (`/app/installments/:id`): ringkasan (sisa pokok, total bunga, lunas k/n) + tabel jadwal + tombol "Bayar angsuran ke-k".
- **Form transaksi dan Quick Add:** kartu/paylater ikut di daftar "Dibayar dari" untuk pengeluaran dan di transfer; transfer punya field "Biaya admin" (opsional).
- **Ringkasan:** kartu "Tagihan segera jatuh tempo" bila ada sisa tagihan ≤ 7 hari lagi atau terlambat.
- ⓘ baru: `statement`, `minimumPayment`, `installment`, `annuity`, `flat`.

## Test
- `installment/schedule_test.go`: anuitas Rp1.000.000, 12%/tahun, 12 bulan → angsuran Rp88.848,79, bunga bulan 1 Rp10.000, total pokok tepat; flat; bunga 0%; tanggal 31 → akhir Februari.
- `service/card_test.go`: siklus (tanggal cetak, jatuh tempo lintas bulan), minimum, status.
- Handler: belanja pakai kartu, transfer berbiaya, bayar tagihan, cicilan dari pencairan sampai lunas, hapus entry angsuran membuka kembali angsurannya.

## Langkah kerja
1. Migration + model.
2. Paket `installment` + test tabel.
3. Aturan entry (Liability sebagai sumber, fee).
4. Kartu: repo + service + siklus + endpoint.
5. Cicilan: repo (bayar dalam satu transaksi DB) + service + endpoint.
6. Test handler, uji Postgres.
7. Frontend: api, halaman kartu & cicilan, form/Quick Add, ringkasan, i18n.
8. Verifikasi browser.
