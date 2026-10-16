# Fase 3 — Kontrak implementasi dan pembagian kerja

Disusun oleh planner (Fable) untuk tiga worker paralel. Setiap file hanya disentuh oleh satu worker.

## 1. Kontrak bersama

### Go (W1 → W2)
Package `github.com/pozzdol/backend-saluyu/internal/period`, file `internal/period/period.go`:
```go
type Period struct{ Start, End, Next time.Time } // End inclusive (display), Next = End+1 (SQL half-open); all midnight UTC
func For(d time.Time, startDay int) Period      // startDay 1..28; period containing calendar date d
func (p Period) Label() string                  // "25 Sep – 24 Oct 2026"; cross-year "25 Dec 2026 – 24 Jan 2027"
```
W2 computes `prev = Start.AddDate(0,0,-1)`.

### HTTP API
Semua di `/api/v1`, di balik Auth, dibungkus `{"data": ...}`. Error: `{"error": string, "fields"?: {...}}`.

Bentuk bersama:
```
Period      {start, end, next, prev, label: string}            tanggal YYYY-MM-DD; prev = start - 1 hari
AmountRow   {account_id, code, name: string, amount: int}       sen
BudgetRow   {account_id, name: string, budget, actual, remaining: int, over, inherited: bool}
BalanceRow  {account_id, code, name, category: string, balance: int}
```
`?date=YYYY-MM-DD` opsional di semua endpoint (default hari ini di server, UTC; frontend selalu mengirimnya).
Tanggal salah → 400 `{"error":"invalid query parameter","fields":{"date":"is invalid"}}`.

| Method/Path | Request | Response `data` |
|---|---|---|
| GET `/dashboard?date=` | — | `{period: Period, income, expense, surplus: int, savings_rate: number (0..1, 0 bila income ≤ 0), to_savings: AmountRow[], to_savings_total: int, budgets: BudgetRow[] (semua akun Expense aktif, urut kode, budget 0 bila belum ada), balances: BalanceRow[] (semua akun Asset aktif, urut kode, per period.end berdasarkan entry_date)}` |
| GET `/budgets?date=` | — | `{period: Period, rows: BudgetRow[]}` (baris sama dengan dashboard.budgets) |
| PUT `/budgets/:id` (id = account_id) | `{"date":"2026-10-16","amount":150000000}` amount ≥ 0 sen; server snap ke period.start | 200 `BudgetRow` (inherited=false). 404 akun tidak ada/bukan milik user; 400 `budgets are only for expense accounts`; 400 validasi |
| GET `/reports/income-statement?date=` | — | `{period: Period, revenue: AmountRow[], revenue_total, expense: AmountRow[], expense_total, net_income: int}` (COALESCE(period_date, entry_date) di [start, next); baris nol dihilangkan) |
| GET `/reports/balance-sheet?date=` | — | `{as_of: string, assets: AmountRow[], assets_total, liabilities: AmountRow[], liabilities_total, equity: AmountRow[], equity_total, current_earnings: int, balanced: bool}` (entry_date ≤ as_of; baris nol dihilangkan; current_earnings = Σrevenue − Σexpense sepanjang waktu sampai as_of) |
| POST `/entries` (diperluas) | tambah `"next_period": bool` (opsional), hanya berlaku untuk kind=income: `period_date = period.For(entry_date, user.period_start_day).Next` | Entry dapat field `"period_date": string\|null` |
| PATCH `/me` (diperluas) | tambah `"period_start_day": int` 1..28 opsional | `UserResponse` dapat `"period_start_day": int` (juga di /login, /me, /register) |

Aturan agregasi (service):
- income = Σ Revenue (credit − debit); expense = Σ Expense (debit − credit), semua kind.
- to_savings = akun Asset dengan category ∈ {Tabungan, Investasi}, kind = transfer, amount = debit − credit (net masuk); baris 0 dihilangkan.
- surplus = income − expense.
- BudgetRow: actual = expense akun itu di periode; remaining = budget − actual; over = actual > budget; inherited = budget efektif berasal dari period_start < period.start.

## 2. W1 (haiku) — migration + package period
Hanya 6 file ini:

`db/migration/000007_add_period_settings.up.sql`
```sql
-- Payday cycle start (1..28: exists in every month, no clamping needed).
ALTER TABLE users ADD COLUMN period_start_day SMALLINT NOT NULL DEFAULT 25
    CHECK (period_start_day BETWEEN 1 AND 28);
-- Date used by period reports (dashboard, income statement). NULL = entry_date. Set when salary
-- arrives early for the next period; entry_date keeps the real bank date (balance sheet uses it).
ALTER TABLE journal_entries ADD COLUMN period_date DATE;
```
`db/migration/000007_add_period_settings.down.sql`
```sql
ALTER TABLE journal_entries DROP COLUMN period_date;
ALTER TABLE users DROP COLUMN period_start_day;
```
`db/migration/000008_create_budgets_table.up.sql`
```sql
CREATE TABLE budgets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id   UUID NOT NULL REFERENCES accounts(id),
    period_start DATE NOT NULL,
    amount       BIGINT NOT NULL CHECK (amount >= 0), -- sen
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, account_id, period_start)
);
-- Serves DISTINCT ON "latest budget with period_start <= this period" (budgets inherit forward without copying).
CREATE INDEX budgets_lookup_idx ON budgets (user_id, account_id, period_start DESC);
```
`db/migration/000008_create_budgets_table.down.sql`: `DROP TABLE IF EXISTS budgets;`

`internal/period/period.go` (package `period`, import hanya `time` dan `fmt`):
```go
// For returns the payday period containing d. A period runs from startDay of one month to
// startDay-1 of the next. startDay is 1..28 so it exists in every month.
func For(d time.Time, startDay int) Period {
	y, m, day := d.Date()
	if day < startDay {
		m-- // time.Date normalizes month 0 to December of the previous year
	}
	start := time.Date(y, m, startDay, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(0, 1, 0)
	return Period{Start: start, End: next.AddDate(0, 0, -1), Next: next}
}
func (p Period) Label() string {
	if p.Start.Year() == p.End.Year() {
		return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan"), p.End.Format("2 Jan 2006"))
	}
	return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan 2006"), p.End.Format("2 Jan 2006"))
}
```
`internal/period/period_test.go`: satu table test `TestFor` dengan `[]struct{ date string; startDay int; start, end, label string }`, parse via `time.Parse(time.DateOnly, …)`, bandingkan `Start`/`End` via `.Format(time.DateOnly)`, cek `Next == End + 1 hari`, cek `Label()`:

| date | startDay | start | end | label |
|---|---|---|---|---|
| 2026-10-16 | 25 | 2026-09-25 | 2026-10-24 | `25 Sep – 24 Oct 2026` |
| 2026-10-25 | 25 | 2026-10-25 | 2026-11-24 | `25 Oct – 24 Nov 2026` |
| 2026-10-24 | 25 | 2026-09-25 | 2026-10-24 | `25 Sep – 24 Oct 2026` |
| 2027-01-10 | 25 | 2026-12-25 | 2027-01-24 | `25 Dec 2026 – 24 Jan 2027` |
| 2026-03-01 | 28 | 2026-02-28 | 2026-03-27 | `28 Feb – 27 Mar 2026` |
| 2028-02-28 | 28 | 2028-02-28 | 2028-03-27 | `28 Feb – 27 Mar 2028` |
| 2026-05-31 | 1 | 2026-05-01 | 2026-05-31 | `1 May – 31 May 2026` |
| 2026-10-16 | 10 | 2026-10-10 | 2026-11-09 | `10 Oct – 9 Nov 2026` |

Selesai bila: `go vet ./internal/period/ && go test ./internal/period/` hijau; empat file SQL persis seperti di atas.

## 3. W2 (opus) — backend inti
Boleh berasumsi file W1 ada. Jangan sentuh `db/migration/*` atau `internal/period/*`.

- **`internal/model/user.go`**: `User.PeriodStartDay int` (`json:"-"`); `UserResponse.PeriodStartDay int` (`json:"period_start_day"`, juga di ToUserResponse); `UpdateProfileRequest.PeriodStartDay *int` (`json:"period_start_day" binding:"omitnil,min=1,max=28"`).
- **`internal/model/ledger.go`**: `Entry` tambah `PeriodDate *time.Time` (`json:"-"`) dan `PeriodDateStr *string` (`json:"period_date"`); `PostEntryRequest` tambah `NextPeriod bool` (`json:"next_period"`).
- **`internal/model/report.go`** (baru): `PeriodJSON{Start,End,Next,Prev,Label string}`, `AmountRow`, `BudgetRow`, `BalanceRow`, `Dashboard`, `BudgetList{Period PeriodJSON; Rows []BudgetRow}`, `BudgetRequest{Date string binding:"required,datetime=2006-01-02"; Amount int64 binding:"gte=0"}`, `IncomeStatement`, `BalanceSheet`, plus baris repo `Movement{AccountID uuid.UUID; Code,Name,Type,Category,Kind string; Debit,Credit int64}` dan `Budget{AccountID uuid.UUID; Name string; PeriodStart time.Time; Amount int64}`.
- **`internal/repository/user.go`**: `period_start_day` di `userColumns` + `scanUser`; `Create` RETURNING menambah `period_start_day`; ubah `Update(ctx, id uuid.UUID, req model.UpdateProfileRequest)` dengan `period_start_day = COALESCE($4, period_start_day)`.
- **`internal/service/user.go`**: signature `UserRepository.Update` mengikuti; `UpdateProfile` meneruskan `req`.
- **`internal/repository/ledger.go`**: `insertEntry` menyimpan `period_date`; `ListEntries` memilih `e.period_date` dan mengisi `PeriodDateStr`; tambah `PeriodStartDay(ctx, userID) (int, error)`.
- **`internal/repository/report.go`** (baru, method pada `*LedgerRepository`):
  - `Movements(ctx, userID, from, to time.Time, byPeriodDate bool) ([]model.Movement, error)`: jumlah per akun per kind di [from, to). byPeriodDate memakai `COALESCE(period_date, entry_date)`, selain itu `entry_date`. `time.Time{}` sebagai from = sepanjang waktu. Cast `$4::bool`.
  - `EffectiveBudgets(ctx, userID, periodStart) ([]model.Budget, error)`: `DISTINCT ON (b.account_id) ... WHERE b.period_start <= $2 ORDER BY b.account_id, b.period_start DESC`.
  - `UpsertBudget(ctx, userID, accountID, periodStart, amount) error`: `ON CONFLICT (user_id, account_id, period_start) DO UPDATE`.
- **`internal/service/ledger.go`**: tambah `PeriodStartDay` ke interface `LedgerRepository`. Di `PostEntry`: bila `req.NextPeriod && req.Kind == "income"`, `e.PeriodDate = &period.For(date, day).Next` (komentar ponytail: diabaikan untuk kind lain). `validate()` mengisi `e.PeriodDateStr`.
- **`internal/service/report.go`** (baru): `ErrNotExpenseAccount`; interface `ReportRepository` (ListAccounts, FindAccounts, PeriodStartDay, Movements, EffectiveBudgets, UpsertBudget); `ReportService` dengan `Dashboard`, `Budgets`, `SetBudget`, `IncomeStatement`, `BalanceSheet`. Helper `periodOf` dan `budgetRows` dipakai bersama. Balance sheet: `balanced = assets_total == liabilities_total + equity_total + current_earnings`.
- **`internal/handler/report.go`** (baru): `ReportHandler` + 5 method; tanggal via `queryDate(c, "date")`/`badQuery`; `pathID` untuk `:id`; `bindJSON` untuk BudgetRequest.
- **`internal/handler/routes.go`**: `RegisterRoutes(r, users, ledger, reports *ReportHandler, tm)`; `GET /dashboard`, `GET /budgets`, `PUT /budgets/:id`, `GET /reports/income-statement`, `GET /reports/balance-sheet`.
- **`internal/handler/user.go` fail()**: `service.ErrNotExpenseAccount` → 400.
- **`internal/handler/bind.go` fieldMessage**: untuk `min`/`max` pada field non-string, pesan tanpa kata "characters".
- **`cmd/api/main.go`**: satu `ledgerRepo` dipakai ledger dan report service.
- **Test** (`internal/handler/`): fake di `user_test.go` dan `ledger_test.go` diperluas (startDay, budgets, Movements, EffectiveBudgets, UpsertBudget, Update dengan period_start_day); `report_test.go` baru dengan `TestDashboardAndReports` dan `TestNextPeriod`; test profil `period_start_day` (30 → 400, 10 → 200 dan dashboard mulai tanggal 10).

Selesai bila: `go vet ./... && go test ./...` hijau.

## 4. W3 (sonnet) — frontend
File: `src/lib/api.ts`, `src/components/period-picker.tsx` (baru), `src/components/field.tsx`, `src/components/app-shell.tsx`, `src/components/site.tsx`, `src/pages/dashboard.tsx`, `src/pages/budgets.tsx`, `src/pages/reports.tsx` (baru), `src/pages/transactions.tsx`, `src/pages/account.tsx`, `src/pages/auth.tsx`, `src/pages/opening.tsx`, `src/app.tsx`. Teks UI bahasa Inggris; uang lewat `fmtIDR`/`toSen`; tanpa dependency baru.

- **api.ts**: `User.period_start_day: number`; `updateProfile` menerima `period_start_day?`; `Entry.period_date: string | null`; `PostEntryInput.next_period?: boolean`; tipe `Period, AmountRow, BudgetRow, BalanceRow, Dashboard, BudgetList, IncomeStatement, BalanceSheet` persis kontrak; `getDashboard`, `getBudgets`, `putBudget`, `getIncomeStatement`, `getBalanceSheet`.
- **period-picker.tsx**: `usePeriodDate()` membaca `?date=` (default `todayISO()`), setter mempertahankan query lain (misal `view`); `PeriodPicker` dengan tombol ‹ (ke `prev`), label, › (ke `next`), dan `<input type="date">`.
- **dashboard.tsx** (`/app/dashboard`): 4 stat tile, daftar "To savings", bar budget `role="meter"` (merah bila over), tautan "Set budgets", tabel saldo aset.
- **budgets.tsx** (`/app/budgets`): tabel budget per akun Expense dengan input + Save per baris, badge "Inherited", catatan bahwa budget terbawa ke periode berikutnya.
- **reports.tsx** (`/app/reports?view=pl|bs`): tab Laba Rugi (periode) dan Neraca (as of), badge balanced.
- **transactions.tsx**: tanggal jadi controlled; checkbox "Count toward next period" hanya untuk income dan hanya bila ≤ 3 hari sebelum tanggal mulai periode; kirim `next_period`; tanda "→ next period" di jurnal bila `period_date`.
- **account.tsx**: field "Payday (period start day)" 1..28 + catatan "Changing this re-slices past periods too."; `field.tsx` tambah `min`/`max`.
- **Navigasi**: Dashboard, Transactions, Budgets, Reports, Accounts, Reconcile, Trial balance, Opening balance, Profile. Redirect `/app`, link Console, setelah login, dan setelah simpan saldo awal → `/app/dashboard`.

Selesai bila: `npx tsc -b && npm run build` berhasil.

## 5. Checklist integrasi (koordinator)
1. `go vet ./... && go test ./...` setelah W1 + W2.
2. `migrate up`, cek kolom/tabel baru, `migrate down 2` lalu `up`.
3. API + frontend jalan; atur payday di profil.
4. Dashboard 16 Okt 2026: label "25 Sep – 24 Oct 2026"; income = Gaji + Bunga; transfer ke Tabungan bukan expense; navigasi ‹ ›.
5. Budget Makan; periode berikutnya "Inherited"; bar merah saat over.
6. Gaji 24 Okt dengan "next period": tidak di periode Sep–Okt, ada di Okt–Nov, saldo Bank 24 Okt tetap memuatnya.
7. Laba Rugi net = surplus dashboard; Neraca balanced; Trial balance tidak berubah.
8. Checkbox next period tersembunyi untuk expense/transfer.
9. Commit.
