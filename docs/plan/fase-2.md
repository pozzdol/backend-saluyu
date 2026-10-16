# Fase 2 — Saldo Awal, Hitung Kas & Rekonsiliasi

Bergantung pada Fase 1: migration terakhir `000005`, fungsi posting generik `post(ctx, userID, entry, lines)` di `LedgerService` (satu tx, cek balance), `LedgerHandler`, fake ledger repo di test. Lihat README untuk kontrak lintas fase.

## Tujuan & definisi selesai (testable)
1. User baru (`start_date` NULL) diarahkan ke `/onboarding`; submit membuat **satu** entry `opening` tanggal `start_date`, lawan 3001. Trial balance seimbang; saldo tiap akun = input.
2. `PUT /opening` kedua kali: sukses jika belum ada entry lain (entry lama diganti); 409 jika sudah ada.
3. Entry apa pun dengan `entry_date < start_date` → 422; tanpa opening → 409.
4. Hitung kas: fisik < buku → Dr 5199 / Cr Kas; fisik > buku → Dr Kas / Cr 4199 Pendapatan Lain-lain; selisih 0 → tidak ada entry. Saldo Kas setelahnya = fisik.
5. Rekonsiliasi bank/e-wallet/utang: preview selisih, konfirmasi → entry `adjustment` ke akun lawan pilihan user.

## Di luar cakupan
Import Excel, multi-opening per akun, rekonsiliasi per-transaksi (matching), riwayat hitung kas terpisah (cukup filter journal `kind=cash_count`).

## Database
`db/migration/000006_add_users_start_date.up.sql`
```sql
-- NULL = buku belum dibuka. Semua entry non-opening wajib entry_date >= start_date (guard di service.post).
ALTER TABLE users ADD COLUMN start_date DATE;
-- Satu opening per user: DB yang menjaga, bukan kode.
CREATE UNIQUE INDEX journal_entries_one_opening_idx ON journal_entries (user_id) WHERE kind = 'opening';
```
`.down.sql`
```sql
DROP INDEX journal_entries_one_opening_idx;
ALTER TABLE users DROP COLUMN start_date;
```
Tidak ada tabel baru: hitung kas dan rekonsiliasi **adalah** journal entry. Syarat: `journal_lines.entry_id ... ON DELETE CASCADE` (sudah di Fase 1).

## Backend
**`internal/model/ledger.go`** (tambah) — `User.StartDate *time.Time` + `UserResponse.StartDate *string "start_date"` (format `2006-01-02`); request:
```go
type OpeningItem struct{ AccountID uuid.UUID `binding:"required"`; Amount int64 `binding:"gte=0"` }
type OpeningRequest struct{ StartDate string `json:"start_date" binding:"required,datetime=2006-01-02"`; Items []OpeningItem `binding:"required,min=1,dive"` }
type ReconcileRequest struct{ AccountID uuid.UUID; Date string `binding:"required,datetime=2006-01-02"`; Actual int64 `json:"actual" binding:"gte=0"`; CounterAccountID *uuid.UUID `json:"counter_account_id"` }
```
**`internal/repository/ledger.go`** (tambah 3 fungsi):
- `BalanceAsOf(ctx, userID, accountID, date) (int64, error)` — `SUM(debit)-SUM(credit)` join entries `WHERE entry_date <= $date`; service membalik tanda untuk normal Credit.
- `CountEntriesExcept(ctx, userID, kind) (int, error)`.
- `ReplaceOpening(ctx, userID, startDate, entry, lines) error` — satu tx: `SELECT id FROM users WHERE id=$1 FOR UPDATE` → `DELETE journal_entries WHERE user_id=$1 AND kind='opening'` → insert entry+lines (reuse insert Fase 1 dengan `*sql.Tx`) → `UPDATE users SET start_date=$2`.

**`internal/service/reconcile.go`** — method pada `*LedgerService`, dua fungsi murni (yang dites):
```go
// openingLines: Asset Dr, Liability Cr, 3001 menampung selisih (Cr bila aset > utang, Dr bila sebaliknya). Amount 0 dilewati.
func openingLines(items []openingItem, equityID uuid.UUID) []model.JournalLine
// adjustLines: geser saldo `acc` sebesar diff searah normal_balance-nya; counter sisi lawan. diff 0 → nil.
func adjustLines(acc model.Account, counterID uuid.UUID, diff int64) []model.JournalLine
```
Aturan `adjustLines` (diff = actual − buku, buku bertanda searah normal):

| Akun | diff > 0 | diff < 0 |
|---|---|---|
| Asset (Kas/Bank) | Dr akun, Cr lawan | Dr lawan, Cr akun |
| Liability (2101) | Cr akun, Dr lawan | Dr akun, Cr lawan |

Method:
- `SetOpening`: validasi tiap akun milik user & type Asset/Liability (`ErrNotBalanceSheet`), `CountEntriesExcept(opening) > 0` → `ErrBooksNotEmpty`, build lines, `ReplaceOpening`.
- `Reconcile(userID, req, kind)`: load akun, `BalanceAsOf`, diff; `counter` = req.CounterAccountID, atau untuk hitung kas: 5199 jika diff < 0 dan 4199 jika diff > 0; counter == akun → `ErrSameAccount`; diff 0 → return tanpa post; else `post(kind, description "Hitung kas: fisik X, buku Y")`. `// ponytail: saldo dibaca di luar tx post; buku single-user, race diabaikan.`
- Guard di `post` Fase 1 (satu titik untuk semua caller): `start_date == nil && kind != opening` → `ErrNotOpened`; `entry_date < start_date` → `ErrBeforeStart`.
- `DELETE /entries/:id` (Fase 1) menolak kind `opening` → 409; opening hanya diganti lewat `PUT /opening`.

**Keputusan (disepakati): kas lebih → kredit 4199 Pendapatan Lain-lain, kas kurang → debit 5199.** Saldo beban tidak pernah negatif; uang "ketemu" terlihat terpisah di Laba Rugi.

**`internal/handler/reconcile.go`** + routes di `routes.go` (group `Auth`):

| Method | Path | Request | Response |
|---|---|---|---|
| GET | `/opening` | – | `{"data":{"start_date","entry_id","items":[{account_id,code,name,amount}],"editable":bool}}`; 404 jika belum ada |
| PUT | `/opening` | `OpeningRequest` | 200 sama seperti GET |
| GET | `/accounts/:id/balance?date=` | – | `{"data":{"balance":56000000}}` |
| POST | `/cash-counts` | `{account_id,date,actual}` (counter otomatis 5199/4199, kind `cash_count`) | 201 `{"data":{"book","actual","diff","entry":{...}\|null}}` |
| POST | `/reconciliations` | `ReconcileRequest` (counter wajib, kind `adjustment`) | 201 sama |

`fail()`: `ErrBooksNotEmpty`, `ErrNotOpened` → 409; `ErrBeforeStart`, `ErrNotBalanceSheet`, `ErrSameAccount` → 422; `ErrAccountNotFound` → 404.

## Frontend
- `src/lib/api.ts`: `User.start_date: string|null`; `getOpening`, `setOpening`, `balanceAsOf(id,date)`, `postCashCount`, `postReconcile`; tipe `Account` dari Fase 1. Format uang pakai `fmtIDR`/`toSen` dari `src/lib/utils.ts` (Fase 1).
- `src/app.tsx`: route `/app/onboarding`, `/app/cash-count`, `/app/reconcile`; di `Protected`: `user.start_date==null && path!=="/app/onboarding"` → `route("/app/onboarding")`.
- `src/pages/onboarding.tsx`: satu form (bukan multi-step): `<input type="date">`, tabel akun Asset+Liability dengan `<Input type="number" step="0.01">`, footer live: total aset − total utang = Ekuitas Saldo Awal. Reuse `Field`, `useForm`, `Status` (pindahkan `useForm`/`Status` dari `account.tsx` ke `field.tsx`). Mode edit menampilkan banner jika `editable=false`.
- `src/pages/reconcile.tsx`: `<Reconcile mode="cash"|"bank" />` (pola `AuthPage`): select akun (cash: kode 1002; bank: Asset lain + Liability), tanggal, nominal fisik; tombol "Hitung" → `balanceAsOf` → tampil buku/fisik/selisih (merah/hijau); mode bank menambah select akun lawan (Expense/Revenue, default 5199); tombol "Posting selisih" nonaktif jika diff 0.

## Test
`internal/service/reconcile_test.go` (table-driven, tanpa DB):
1. `TestOpeningLines`: Bank 150000000, Kas 60000000, Kas-2 0, Utang 50000000 → 4 lines; 3001 Cr 160000000; `Σdebit == Σcredit`; akun amount 0 tidak muncul; kasus utang > aset → 3001 di sisi Dr.
2. `TestAdjustLines`: Asset diff −6000000 → `{5199 Dr 6000000},{Kas Cr 6000000}`; diff +100000 → Kas Dr, 4199 Cr; Liability diff +200000 → 2101 Cr, lawan Dr; diff 0 → `nil`.
3. `internal/handler/ledger_test.go` (fake Fase 1 + field `startDate`): POST expense tanpa opening → 409; PUT opening → 200; expense bertanggal sebelum start → 422; PUT opening kedua → 409.

## Langkah kerja berurutan
1. Migration 000006, `migrate up`.
2. Model: `StartDate` di User/UserResponse, request structs; scan `start_date` di `scanUser`.
3. Tulis `openingLines`/`adjustLines` + test 1–2 dulu (merah → hijau).
4. Repo: `BalanceAsOf`, `CountEntriesExcept`, `ReplaceOpening`.
5. Guard `ErrNotOpened`/`ErrBeforeStart` di `post`; service `SetOpening`, `GetOpening`, `Reconcile`.
6. Handler + routes + `fail`; test 3.
7. `api.ts`, redirect di `app.tsx`.
8. `onboarding.tsx`, lalu `reconcile.tsx`.
9. Uji manual: register → onboarding → expense → hitung kas → trial balance seimbang.

## Konsep Go yang dipelajari (↔ Laravel)
- Method pada tipe yang sama tersebar di beberapa file satu package ≈ memecah class dengan trait.
- `*time.Time`/`sql.NullTime` untuk kolom nullable ≈ `?Carbon` cast.
- `tx, _ := db.BeginTx; defer tx.Rollback(); ...; tx.Commit()` ≈ `DB::transaction(fn)`; rollback setelah commit = no-op.
- `SELECT ... FOR UPDATE` ≈ `lockForUpdate()`.
- Partial unique index sebagai validasi ≈ `Rule::unique()->where(...)`, tapi di DB.
- Sentinel error + `errors.Is` ≈ exception class → `Handler::render`.
- `int64` sen tanpa float ≈ `bcmath`/`brick/money`.
- Table-driven test `t.Run` ≈ `@dataProvider`.
- `validator` tag `datetime=2006-01-02`, `dive` ≈ `date_format:Y-m-d`, `items.*.amount`.

## Risiko & pertanyaan terbuka
- `DATE` vs timezone: kirim/terima string `YYYY-MM-DD`, jangan `time.Now()` lokal di server UTC.
- Race saldo vs post (dua tab) sengaja diabaikan; pindahkan `BalanceAsOf` ke dalam tx bila jadi masalah.
- Saldo awal negatif (bank overdraft) ditolak `gte=0` — catat sebagai Liability saja.
- Saldo awal piutang/utang per kontak butuh `contact_id` (kolom baru di Fase 4). Opening Fase 2 belum mendukung; tambahkan saat Fase 4.
