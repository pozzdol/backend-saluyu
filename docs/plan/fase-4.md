# Fase 4 — Kontak & Piutang/Utang per Kontak

Bergantung pada Fase 1: fungsi posting generik `post(ctx, userID, entry, lines)` di `LedgerService` yang memvalidasi debit = kredit + kepemilikan akun dalam satu `sql.Tx`, dan seed akun per user memuat `1101 Piutang` (Asset/Debit) dan `2101 Utang` (Liability/Credit).

## Tujuan & definisi selesai (testable)
- CRUD kontak; nama unik per user case-insensitive ("Bau" vs "bau" → 409).
- `POST /loans` menghasilkan journal entry seimbang untuk Talangan (termasuk split bill 3 baris), Pinjam, Pelunasan masuk/keluar.
- Pelunasan > saldo outstanding → 409; dengan `confirm_overpay:true` sisa dikonversi ke sisi lawan (piutang berlebih jadi utang, dan sebaliknya). **Keputusan: konversi, bukan tolak total**, karena net posisi tetap benar dan tidak butuh entry kedua.
- `GET /contacts` menampilkan receivable/payable/net per kontak; `GET /contacts/:id` + history dengan running balance.
- Hapus kontak bersaldo ≠ 0 → 409.
- Baris 1101/2101 wajib `contact_id`, baris lain dilarang; dicek di `post()`.

## Di luar cakupan
Auto-posting lintas buku + approval (Fase 5; hanya kolom `linked_user_id` disiapkan), set `linked_user_id` via API, arsip kontak, offset/netting piutang-utang otomatis, import histori Excel "Bau".

## Database
`db/migration/000009_create_contacts_table.up.sql`
```sql
CREATE TABLE contacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,
    -- Fase 5: user lain yang bukunya bercermin ke kontak ini. Kolom saja, belum ada perilaku.
    linked_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Pola sama dengan users_email_lower_key.
CREATE UNIQUE INDEX contacts_user_name_lower_key ON contacts (user_id, lower(name));
```
down: `DROP TABLE contacts;`

`db/migration/000010_add_contact_id_to_journal_lines.up.sql`
```sql
ALTER TABLE journal_lines
    ADD COLUMN contact_id UUID REFERENCES contacts(id) ON DELETE RESTRICT;
CREATE INDEX journal_lines_contact_id_idx ON journal_lines (contact_id) WHERE contact_id IS NOT NULL;
```
down: `ALTER TABLE journal_lines DROP COLUMN contact_id;`

**Enforcement contact_id: service, bukan DB.** CHECK constraint tidak bisa lihat `accounts.code` (lintas tabel); alternatifnya trigger PL/pgSQL — kode non-Go yang tidak ikut `go test` dengan fake. Semua tulisan sudah lewat satu chokepoint `post()` yang sudah memuat akun per baris untuk cek kepemilikan, jadi aturannya 3 baris di loop yang sama. `// ponytail: aturan contact_id di post(); trigger kalau muncul penulis kedua (SQL import)`. RESTRICT pada FK tetap menjaga histori.

## Backend
File baru: `internal/model/contact.go`, `internal/repository/contact.go`, `internal/service/contact.go`, `internal/handler/contact.go`, `internal/handler/contact_test.go`. Diubah: `routes.go`, `fail()` di `user.go`, `main.go`, `service/ledger.go` (Fase 1, +4 baris aturan contact_id).

Endpoint (semua di bawah `Auth`, user dari `currentClaims`):

| Method | Path | Request | Response |
|---|---|---|---|
| GET | /contacts | – | `{data:[{id,name,linked_user_id,receivable,payable,net}]}` |
| POST | /contacts | `{name}` | 201 `{data:contact}` |
| PATCH | /contacts/:id | `{name}` | `{data:contact}` |
| DELETE | /contacts/:id | – | 204 |
| GET | /contacts/:id | – | `{data:{contact, balance:{receivable,payable,net}, history:[{entry_id,entry_date,kind,description,account_code,debit,credit,running_net}]}}` |
| POST | /loans | `{type, contact_id, account_id, amount, entry_date, description, my_share?, expense_account_id?, confirm_overpay?}` | 201 `{data:journal_entry}` |

`type` binding `oneof=talangan pinjam lunas_masuk lunas_keluar`; `amount` `gt=0`; `my_share` `gte=0,ltfield=Amount` (hanya talangan; butuh `expense_account_id` bila >0). Uang int64 sen.

Baris jurnal (`[k]` = contact_id terisi):

| Form | kind | Debit | Kredit |
|---|---|---|---|
| Talangan / split bill | loan | 1101[k] amount−my_share; expense my_share (jika >0) | kas/bank amount |
| Pinjam | loan | kas/bank amount | 2101[k] amount |
| Pelunasan masuk | repayment | kas/bank amount | 1101[k] min(amount,receivable); 2101[k] sisa (hanya jika confirm) |
| Pelunasan keluar | repayment | 2101[k] min(amount,payable); 1101[k] sisa (hanya jika confirm) | kas/bank amount |

Service `ContactService` dengan interface `ContactRepository` (Create, List, FindByID, Rename, Delete, Balance, History) dan `Poster { Post(...) }` — `*LedgerService` memenuhinya tanpa `implements`. Alur `/loans`: FindByID kontak (milik user) → Balance → susun lines → `post`. Transaction boundary: `post()` sendiri sudah satu tx. Cek saldo lalu post **tidak** diserialisasi: `// ponytail: check-then-post; buku per user, penulis tunggal — SELECT FOR UPDATE contacts jika Fase 5 menambah penulis kedua`.

Saldo, satu query:
```sql
SELECT COALESCE(SUM(CASE WHEN a.code='1101' THEN l.debit-l.credit ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN a.code='2101' THEN l.credit-l.debit ELSE 0 END),0)
FROM journal_lines l JOIN accounts a ON a.id=l.account_id WHERE l.contact_id=$1
```
net = receivable − payable (positif = dia berutang ke saya). History: join entries, `SUM(l.debit-l.credit) OVER (ORDER BY e.entry_date, e.entry_no) AS running_net`.

Error → HTTP (tambahan di `fail`): `ErrContactNameTaken` 409 (23505), `ErrContactNotFound` 404, `ErrContactHasBalance` 409, `ErrContactHasHistory` 409 (23503 dari RESTRICT), `ErrOverpayment` 409, `ErrContactRequired`/`ErrContactForbidden` 400 (dari post).

## Frontend
- `src/pages/contacts.tsx`: `ContactsPage` (tabel saldo + form tambah) dan `ContactDetail` (kartu net, satu `<form>` dengan `<select name="type">` native, field `my_share`/expense muncul hanya saat talangan, `<input type="date">`, tabel history). Pakai `Field`, `useForm`, `formValues`, `toFormError` dari `field.tsx`.
- Overpay: tangkap `ApiError` 409 → `window.confirm("Melebihi saldo Rp X. Sisa jadi utang?")` → kirim ulang `confirm_overpay:true`. Native dialog, tanpa komponen baru.
- `app.tsx`: route `/app/contacts`, `/app/contacts/:id`.
- `api.ts`: tipe `Contact`, `ContactDetail`, `LoanInput`; fungsi `listContacts`, `createContact`, `renameContact`, `deleteContact`, `getContact`, `postLoan` via `authed`.
- Format uang: `fmtIDR` dari `lib/utils.ts` (Fase 1).

## Test
`internal/handler/contact_test.go`, fake in-memory (`fakeContacts` + `fakeLedger` yang menyimpan lines; `Balance` dihitung dari lines fake), satu `TestContactFlow`:
1. Register/login; POST /contacts "Bau" → 201; "bau" → 409.
2. Split bill: talangan amount 100000, my_share 40000 → 201; assert 3 lines: 1101 debit 60000 contact≠nil, expense debit 40000 contact=nil, kas kredit 100000; ΣD==ΣK; GET detail receivable==60000, net==60000.
3. lunas_masuk 70000 → 409; dengan confirm → lines Cr 1101 60000, Cr 2101 10000; GET: receivable 0, payable 10000, net −10000.
4. DELETE kontak → 409 (saldo ≠ 0).
5. Post baris 1101 tanpa contact lewat `post()` → 400.

## Langkah kerja berurutan
1. Verifikasi seed Fase 1 punya kode 1101/2101.
2. Migrasi 000009, 000010; `migrate up`.
3. `model/contact.go`: Contact, ContactBalance, ContactLine, request structs (+Normalize trim nama).
4. `repository/contact.go`: 7 method, mapping 23505/23503.
5. `service/ledger.go`: aturan contact_id di loop validasi post.
6. `service/contact.go`: CRUD + `PostLoan` (switch type → lines).
7. `handler/contact.go` + routes + `fail`.
8. `contact_test.go` hijau.
9. `api.ts`, `contacts.tsx`, routes.
10. Perluas opening Fase 2: item opening untuk 1101/2101 menerima `contact_id`.

## Konsep Go yang dipelajari (↔ Laravel)
- Interface dipenuhi implisit (`Poster`) ↔ `implements` + service container binding.
- Fake in-memory di `_test.go` ↔ `Mockery::mock` / `RefreshDatabase`.
- `uuid.NullUUID` / pointer untuk kolom nullable ↔ `$fillable` nullable Eloquent.
- `pgconn.PgError` 23505/23503 → sentinel `errors.Is` ↔ `QueryException` + `getCode()`.
- Validator tag `oneof`, `ltfield` ↔ rule `in:`, `lt:field`.
- Window function di SQL mentah ↔ `Collection::reduce` di PHP.

## Risiko & pertanyaan terbuka
- Kontak bersaldo nol tapi punya histori ikut tertolak (RESTRICT, 409). Perlu flag arsip? Ditunda.
- Overpay membuat kontak bisa punya piutang *dan* utang sekaligus; laporan hanya tampilkan net, tidak ada entry offset.
- Race check-then-post diterima karena buku per user; wajib ditinjau ulang di Fase 5.
