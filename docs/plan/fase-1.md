# Fase 1 — Buku besar inti

Talangan/Pelunasan → Fase 4, saldo awal → Fase 2.

## Tujuan & definisi selesai
1. Register user baru otomatis punya 19 akun template (disalin oleh kode Go dalam transaction yang sama dengan insert user); user lama ikut di-seed oleh migration. `GET /accounts` hanya mengembalikan akun milik user.
2. `POST /entries` kind `income|expense|transfer` menghasilkan tepat 2 `journal_lines`, Σdebit = Σcredit, akun dicek tipe & kepemilikan (akun user lain → 404, tipe salah → 400, `from == to` → 400, `amount <= 0` → 400).
3. `GET /entries` paginasi + filter tanggal/kind/akun/teks; `DELETE /entries/:id` menghapus entry + lines (cascade).
4. `GET /accounts/:id/ledger` memberi saldo awal periode, baris dengan running balance mengikuti `normal_balance`, saldo akhir.
5. `GET /reports/trial-balance` → `total_debit == total_credit` selalu (`balanced: true`), dijamin CHECK constraint + aturan service.
6. `go test ./...` hijau, frontend bisa input Pemasukan/Pengeluaran/Transfer dan melihat jurnal, buku besar, neraca saldo.

## Di luar cakupan
Saldo awal, hitung kas, periode gaji, budget, kontak/piutang, grup, edit entry (koreksi = hapus lalu buat ulang), kolom `contact_id` (ditambah Fase 4 bersama FK-nya), refund (lihat Risiko).

## Database

**000004_accounts_per_user.up.sql**
```sql
ALTER TABLE accounts
  ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  ALTER COLUMN code DROP DEFAULT,
  DROP CONSTRAINT accounts_code_key;

-- Seed untuk user yang sudah ada. User baru di-seed oleh Go (repository.SeedAccounts) saat register.
-- Template sama dengan model.DefaultAccounts di Go; migration ini hanya jalan sekali.
INSERT INTO accounts (user_id, code, name, type, category, normal_balance)
SELECT u.id, t.code, t.name, t.type::account_type, t.category,
       CASE WHEN t.type IN ('Asset','Expense') THEN 'Debit' ELSE 'Credit' END::normal_balance
FROM users u CROSS JOIN (VALUES
    ('1001','Bank','Asset','Kas & Bank'), ('1002','Cash','Asset','Kas & Bank'),
    ('1003','Tabungan','Asset','Tabungan'), ('1004','E-wallet','Asset','Kas & Bank'),
    ('1005','Investasi','Asset','Investasi'), ('1101','Piutang','Asset','Piutang'),
    ('2101','Utang','Liability','Utang'), ('3001','Ekuitas Saldo Awal','Equity','Ekuitas'),
    ('4001','Gaji','Revenue','Pendapatan'), ('4002','Bunga','Revenue','Pendapatan'),
    ('4003','Pengembalian','Revenue','Pendapatan Lain'), ('4199','Pendapatan Lain-lain','Revenue','Pendapatan Lain'),
    ('5101','Transportasi','Expense','Beban'), ('5102','Makan','Expense','Beban'),
    ('5103','Jajan','Expense','Beban'), ('5104','Entertainment','Expense','Beban'),
    ('5105','Keluarga','Expense','Beban'), ('5107','Admin','Expense','Beban'),
    ('5199','Beban Tunai Lain-lain','Expense','Beban')
) t(code, name, type, category);
DELETE FROM accounts WHERE user_id IS NULL;      -- baris global lama (data dev, tidak ada jurnal)
ALTER TABLE accounts ALTER COLUMN user_id SET NOT NULL,
  ADD CONSTRAINT accounts_user_code_key UNIQUE (user_id, code);
```
Down: drop constraint, kolom `user_id`; `ADD UNIQUE (code)`.

**Seed user baru di Go (standar industri: template di kode aplikasi, bukan trigger DB).** `model.DefaultAccounts` = slice 19 akun di atas. `UserRepository.Create` dijalankan dalam tx: insert user → `SeedAccounts(tx, userID)` (satu `INSERT ... VALUES` multi-row) → commit. Register gagal = tidak ada akun yatim. Test: register lewat fake → `GET /accounts` mengembalikan 19 akun.

**000005_create_journal_tables.up.sql**
```sql
CREATE TYPE entry_kind AS ENUM ('income','expense','transfer','loan','repayment','opening','cash_count','adjustment');

CREATE TABLE journal_entries (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entry_no    BIGINT NOT NULL,
  entry_date  DATE NOT NULL,
  kind        entry_kind NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, entry_no)
);
CREATE INDEX journal_entries_user_date_idx ON journal_entries (user_id, entry_date DESC, entry_no DESC);

CREATE TABLE journal_lines (
  id         BIGSERIAL PRIMARY KEY,                 -- urutan baris stabil untuk running balance
  entry_id   UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
  account_id UUID NOT NULL REFERENCES accounts(id),
  debit      BIGINT NOT NULL DEFAULT 0,             -- sen
  credit     BIGINT NOT NULL DEFAULT 0,
  CHECK (debit >= 0 AND credit >= 0 AND (debit = 0 OR credit = 0) AND debit <> credit)
);
CREATE INDEX journal_lines_account_idx ON journal_lines (account_id, entry_id);
```
Down: drop kedua tabel, drop type.

## Backend

File: `internal/model/ledger.go`, `internal/repository/ledger.go`, `internal/service/ledger.go`, `internal/handler/ledger.go`, `internal/handler/ledger_test.go`; ubah `routes.go` (tambah param `*LedgerHandler`), `user.go` (`fail` tambah case), `cmd/api/main.go` (wiring), `user_test.go` (`newTestServer` ikut signature baru).

Endpoint (semua di `/api/v1`, middleware `Auth`):

| Method | Path | Request | Response |
|---|---|---|---|
| GET | `/accounts?type=&active=` | – | `{data:[{id,code,name,type,category,normal_balance,is_active}]}` |
| POST | `/accounts` | `{code,name,type,category}` | 201 akun; 409 kode dobel |
| PATCH | `/accounts/:id` | `{name?,category?,is_active?}` | akun |
| GET | `/accounts/:id/ledger?from=&to=` | – | `{data:{account, opening, rows:[{entry_id,entry_no,entry_date,kind,description,counter,debit,credit,balance}], closing}}` |
| POST | `/entries` | `{kind,entry_date:"2026-10-16",amount,from_account_id,to_account_id,description}` | 201 entry + lines |
| GET | `/entries?from=&to=&kind=&account_id=&q=&page=1&limit=50` | – | `{data:[entry{...,lines:[{account_id,code,name,debit,credit}]}],total,page,limit}` |
| DELETE | `/entries/:id` | – | 204; 404 bila bukan milik user |
| GET | `/reports/trial-balance?as_of=` | – | `{data:{rows:[{code,name,type,debit,credit,debit_balance,credit_balance}],total_debit,total_credit,balanced}}` |

Service rules (`LedgerService.PostEntry`):
- **Kontrak lintas fase:** `PostEntry` (form from/to) hanya membangun 2 lines lalu memanggil `post(ctx, userID, entry, lines)` — fungsi generik berbasis lines yang memvalidasi kepemilikan akun, Σdebit = Σcredit, lalu memanggil repo `CreateEntry` dalam satu tx. Fase 2 (opening, hitung kas), Fase 4 (split bill 3 lines, contact_id) dan Fase 5 (mirror entry) memakai `post` yang sama.
- Satu aturan untuk semua kind: **Dr `to_account` / Cr `from_account`** sebesar `amount` (uang mengalir dari `from` ke `to`). Kind hanya menentukan tipe yang sah:
  - `income`: from = Revenue, to = Asset (Dr Bank / Cr Gaji)
  - `expense`: from = Asset, to = Expense (Dr Makan / Cr Bank)
  - `transfer`: from = Asset, to = Asset, `from != to`
- Validasi binding: `kind oneof=income expense transfer`, `entry_date datetime=2006-01-02`, `amount gt=0` (int64 sen), `description max=200` + trim.
- Repo `FindAccounts(ctx, userID, ids)` → kurang dari 2 akun aktif → `ErrAccountNotFound`; service cek tipe → `ErrWrongAccountType`. Sebelum simpan: `assert sumDebit == sumCredit` → `ErrUnbalanced` (500, bug internal).
- `normal_balance` diturunkan di service: Asset/Expense → Debit, lainnya Credit.
- Tx boundary: `CreateEntry` di repo: `BeginTx` → `SELECT pg_advisory_xact_lock(hashtext($user_id))` → `INSERT journal_entries ... entry_no = (SELECT COALESCE(MAX(entry_no),0)+1 ... WHERE user_id=$1) RETURNING id, entry_no` → insert 2 lines → `Commit`. Lock mencegah dua request user yang sama bentrok di `entry_no`.
- Filter list memakai param NULL-able (`($3::date IS NULL OR e.entry_date >= $3)`), `COUNT(*) OVER()` untuk total, lines via `json_agg` lalu `json.Unmarshal` — satu query.
- Ledger: query 1 = `opening` (Σ sebelum `from`), query 2 = baris dengan `SUM(CASE normal_balance ...) OVER (ORDER BY entry_date, entry_no, l.id)` + `opening`; `counter` = akun lawan via subquery.
- Trial balance: `accounts LEFT JOIN (lines JOIN entries WHERE entry_date <= as_of)`, `GROUP BY a.id`; service hitung `debit_balance/credit_balance` dan `balanced`.

Error → HTTP: `ErrAccountNotFound`, `ErrEntryNotFound` 404; `ErrAccountCodeTaken` 409 (pgErr 23505); `ErrWrongAccountType`, `ErrSameAccount` 400; lainnya 500.

## Frontend
- `src/lib/api.ts`: type `Account`, `Entry`, `Line`, `LedgerRow`, `TrialBalance`; fungsi `listAccounts`, `createAccount`, `updateAccount`, `createEntry`, `listEntries(params)`, `deleteEntry`, `getLedger`, `getTrialBalance`.
- `src/lib/utils.ts`: helper uang bersama untuk semua fase: `fmtIDR(sen)` (`Intl.NumberFormat("id-ID",{style:"currency",currency:"IDR"})`, `/100`) dan `toSen(input)` (`Math.round(Number(v)*100)`).
- `src/components/app-shell.tsx`: sidebar nav (Transaksi, Akun, Neraca Saldo, Akun saya), header gradien biru muda, konten `max-w-6xl`.
- `src/app.tsx`: `Protected` digeneralisasi sekali di sini (menerima `component`), dipakai semua fase; route `/app/transactions`, `/app/accounts`, `/app/accounts/:id`, `/app/trial-balance`; `/account` tetap.
- `src/pages/transactions.tsx`: tab Pemasukan / Pengeluaran / Transfer (satu form, `kind` state, `<select>` native diisi dari `listAccounts` difilter tipe, `<input type="date">`, `<input type="number" step="0.01">`), di bawahnya tabel jurnal + filter + paginasi + tombol hapus (`confirm()`).
- `src/pages/accounts.tsx`: tabel COA per tipe, form tambah akun inline, toggle aktif, link ke buku besar.
- `src/pages/ledger.tsx`: filter from/to, tabel opening → rows (running balance) → closing.
- `src/pages/trial-balance.tsx`: tabel + footer total, badge hijau "Seimbang".
- Tidak ada dependency baru; `<table>`/`<select>` native + Tailwind.

## Test
`internal/handler/ledger_test.go` — `fakeLedger` in-memory (`accounts map`, `entries slice`) implement `service.LedgerRepository`; register user via `newTestServer` (fake user repo di-seed 4 akun: Bank, Tabungan, Gaji, Makan). `TestPostEntry`:
- income 5.000.000 → 201, 2 lines, Dr Bank 500000000 / Cr Gaji; expense 25.000 → Dr Makan / Cr Bank; transfer 1.000.000 Bank→Tabungan → Dr Tabungan / Cr Bank; tiap entry Σdebit == Σcredit, `entry_no` 1,2,3.
- expense dengan `to` = Gaji → 400; transfer from==to → 400; amount 0 → 400; account_id user lain → 404; DELETE lalu GET → total berkurang.

Opsional `internal/repository/ledger_test.go` gated `TEST_DB_SOURCE` (skip bila kosong): post 3 entry nyata, assert trial balance `total_debit == total_credit` dan running balance ledger Bank = 3.975.000.

## Langkah kerja berurutan
1. Tulis 000004 + 000005, `migrate up`, cek `SELECT count(*) FROM accounts GROUP BY user_id` = 19.
2. `model/ledger.go`: struct + request dengan binding tag; `go build`.
3. `repository/ledger.go`: `ListAccounts`, `CreateAccount`, `UpdateAccount`, `FindAccounts`; `go build`.
4. `service/ledger.go`: interface, errors, `PostEntry` (aturan lines) — tanpa repo entry dulu; tulis `ledger_test.go` + fake; `go test` hijau untuk aturan lines.
5. Repo `CreateEntry` (tx + advisory lock), `ListEntries`, `DeleteEntry`; handler + routes + `fail`; wiring main; `go test ./...`.
6. Repo `Ledger`, `TrialBalance` + handler; verifikasi manual via curl.
7. Frontend: `api.ts` → app-shell + routes → `accounts.tsx` → `transactions.tsx` → `ledger.tsx` → `trial-balance.tsx`. `tsc -b` tiap langkah.

## Konsep Go yang dipelajari (↔ Laravel)
- `db.BeginTx / tx.Commit / defer tx.Rollback()` ↔ `DB::transaction()`.
- `pg_advisory_xact_lock` ↔ `lockForUpdate()` tanpa baris.
- Parameter NULL-able (`*time.Time`, `*string`) untuk filter opsional ↔ `->when($from, fn...)`.
- `rows.Next()/Scan` + scan `[]byte` JSON lalu `json.Unmarshal` ↔ Eloquent hydrate + `with('lines')`.
- `errors.As(err, &pgErr)` kode 23505 ↔ `QueryException` unique.
- Interface di consumer + fake in-memory ↔ Contract + `Mockery`.
- `validator oneof/datetime/gt` ↔ `Rule::in`, `date_format`, `gt:0`.
- `int64` sen tanpa float ↔ `bcmath`/cents di PHP.

## Risiko & pertanyaan terbuka
- Template COA ada di dua tempat: migration 000004 (sekali jalan untuk user lama) dan `model.DefaultAccounts`. Setelah 000004 dijalankan, hanya versi Go yang dipakai.
- Refund belanja: **diputuskan** dicatat sebagai `income` ke 4003 Pengembalian (form Pemasukan biasa). Beban tidak pernah dikredit oleh form, jadi saldo beban tidak bisa negatif.
- `DELETE /entries` berlaku semua kind; Fase 2 menolak `opening`, Fase 5 menolak entry yang punya loan request approved.
- JSON `amount` sebagai number: aman sampai 2^53 sen (~90 triliun rupiah), cukup.
- `code` masih `VARCHAR(100)`; tidak dipersempit agar migration kecil.
