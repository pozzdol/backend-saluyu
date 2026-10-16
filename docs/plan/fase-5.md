# Fase 5 — Grup Berbagi, Linking Kontak, dan Loan Request

## Tujuan & definisi selesai (testable)
1. User bisa membuat grup, mengundang via email, menerima/menolak/keluar. Grup "aktif" bagi member hanya setelah accept.
2. Undangan disimpan per **email**, bukan per user. Pengundang melihat daftar "menunggu: email" untuk semua undangan, terdaftar atau tidak, jadi daftar ini tidak membocorkan email mana yang punya akun. Response invite identik (201, body sama) untuk semua kasus. User yang mendaftar belakangan dengan email itu langsung melihat undangannya.
3. `GET /groups/:id/summary` hanya mengembalikan angka agregat (summary service Fase 3) untuk member *active*; pemanggil yang bukan member active → 404. Setelah `leave`, request yang sama → 404 seketika.
4. Link kontak ke user lain gagal (404) jika kedua user tidak berbagi grup aktif.
5. Posting entry `loan`/`repayment` ke kontak ber-`linked_user_id` otomatis membuat `loan_requests` pending (satu per entry — UNIQUE). Approve 2x / paralel → tepat satu mirror entry; semua call mengembalikan `to_entry_id` yang sama.
6. Mirror entry balance (Σdebit = Σcredit), contact line di 1101/2101 milik approver, description generik (tidak menyalin milik A).
7. Edit/delete entry asal setelah approved → 409 (**forbid**, bukan reverse). Pending/rejected → delete entry ikut menghapus request-nya.
8. `GET /inbox` mengembalikan invite + loan request pending untuk user login; UI polling 30 detik, badge di header.

## Di luar cakupan
Email sungguhan (invite in-app saja), role admin grup, hapus grup, reverse otomatis dua buku, websocket, notifikasi persist/read-state, riwayat loan request lama (pakai list entry + badge status).

## Database
**`db/migration/000011_create_groups.up.sql`**
```sql
CREATE TYPE group_member_status AS ENUM ('active','left');

CREATE TABLE groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE group_members (
    group_id     UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status       group_member_status NOT NULL DEFAULT 'active',
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at      TIMESTAMPTZ,
    PRIMARY KEY (group_id, user_id)          -- satu baris per user per grup; join ulang = upsert
);
CREATE INDEX group_members_user_status_idx ON group_members (user_id, status);

-- Undangan per email: email yang belum terdaftar pun tersimpan, jadi daftar "menunggu"
-- terlihat sama untuk email terdaftar dan tidak (tidak membocorkan siapa yang punya akun).
CREATE TABLE group_invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    email       VARCHAR(100) NOT NULL,          -- disimpan lowercase (model.NormalizeEmail)
    invited_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, email)
);
CREATE INDEX group_invites_email_idx ON group_invites (email);
```
Down: `DROP TABLE group_invites; DROP TABLE group_members; DROP TABLE groups; DROP TYPE group_member_status;`

**`db/migration/000012_create_loan_requests.up.sql`**
```sql
CREATE TYPE loan_request_status AS ENUM ('pending','approved','rejected');

CREATE TABLE loan_requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- UNIQUE: satu request per entry asal. ON DELETE RESTRICT: entry yang sudah approved terkunci di level DB.
    from_entry_id UUID NOT NULL UNIQUE REFERENCES journal_entries(id) ON DELETE RESTRICT,
    from_user_id  UUID NOT NULL REFERENCES users(id),
    to_user_id    UUID NOT NULL REFERENCES users(id),
    amount        BIGINT NOT NULL CHECK (amount > 0),
    status        loan_request_status NOT NULL DEFAULT 'pending',
    to_entry_id   UUID UNIQUE REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at  TIMESTAMPTZ,
    CHECK (from_user_id <> to_user_id),
    CHECK ((status = 'approved') = (to_entry_id IS NOT NULL))
);
CREATE INDEX loan_requests_to_user_pending_idx ON loan_requests (to_user_id) WHERE status = 'pending';
```

## Backend
File baru: `internal/model/group.go`, `internal/repository/group.go`, `internal/repository/loan.go`, `internal/service/share.go` (satu service: grup + loan + inbox), `internal/handler/share.go`. Edit: `routes.go`, `main.go`, `handler/fail`, plus hook kecil di journal service/repo (Fase 1) dan contact service (Fase 4).

**Aturan akses tunggal** (`GroupRepository.SharesActiveGroup(ctx, a, b uuid.UUID) (bool, error)`):
```sql
SELECT EXISTS (SELECT 1 FROM group_members a JOIN group_members b ON a.group_id = b.group_id
  WHERE a.user_id=$1 AND b.user_id=$2 AND a.status='active' AND b.status='active')
```
Semua cek berbasis `currentClaims(c).UserID`; ID dari path tidak pernah dipercaya tanpa join ke user login. Gagal otorisasi → `ErrNotFound` (404, bukan 403, agar keberadaan grup/user tidak bocor).

| Method/Path | Request → Response | Otorisasi |
|---|---|---|
| `POST /groups` | `{name}` → 201 `{data: group}`; creator langsung `active` (satu tx) | login |
| `GET /groups` | → `{data: [{id,name,members:[{id,name,img_url}],pending_invites:[{id,email,created_at}]}]}` hanya grup tempat caller `active` | login |
| `POST /groups/:id/invite` | `{email}` → **selalu 201** `{data:{id,email}}`; `INSERT … ON CONFLICT (group_id,email) DO NOTHING` | caller `active` di grup; jika tidak → 404 |
| `DELETE /groups/:id/invites/:invite_id` | → 204 (batalkan undangan) | caller `active` di grup |
| `POST /invites/:id/accept` / `reject` | → 204. accept (satu tx): hapus invite `WHERE id=$1 AND email=<email caller>`, upsert `group_members` status `active`; reject = hapus invite; 0 row → 404 | email invite = email caller |
| `POST /groups/:id/leave` | → 204 | `UPDATE … SET status='left', left_at=now() WHERE … status='active'`; 0 row → 404 |
| `GET /groups/:id/summary?date=YYYY-MM-DD` | → `{data:[{user:{id,name}, summary}]}` memanggil `ForUser(memberID, date)` Fase 3 per member active (periode mengikuti `period_start_day` masing-masing member) | caller `active` di grup |
| `GET /inbox` | → `{invites:[{id,group_id,group_name,invited_by_name}], loans:[{id,from_name,kind,amount,created_at}]}`; invite dicari `WHERE email = <email caller>` | login |
| `POST /loan-requests/:id/approve` | `{account_id}` → 200 `{to_entry_id}` | `to_user_id=me`; `account_id` harus `accounts.user_id=me` (400 jika tidak) |
| `POST /loan-requests/:id/reject` | → 204 | `to_user_id=me AND status='pending'` |
| `PATCH /contacts/:id` (Fase 4) | tambah `linked_user_id`: harus ≠ me dan `SharesActiveGroup` → else 404 | owner kontak |

Invite: `RateLimit(20, time.Hour)`. Server **tidak** memanggil `FindByEmail` saat invite: email langsung disimpan apa adanya (lowercase), sehingga respons dan daftar "menunggu" sama persis untuk email terdaftar maupun tidak. Mengundang email sendiri atau email anggota aktif → tetap disimpan; saat accept, upsert member membuatnya no-op. 1:1 match = grup beranggota dua; tidak ada konsep terpisah.

**Pembuatan loan request** — di `JournalRepository.Create` (Fase 1) dalam tx yang sama, satu statement setelah insert lines:
```sql
INSERT INTO loan_requests (from_entry_id, from_user_id, to_user_id, amount)
SELECT e.id, e.user_id, c.linked_user_id, GREATEST(l.debit, l.credit)
FROM journal_entries e JOIN journal_lines l ON l.entry_id=e.id JOIN contacts c ON c.id=l.contact_id
WHERE e.id=$1 AND e.kind IN ('loan','repayment') AND c.linked_user_id IS NOT NULL
```
Service journal sebelumnya memastikan `SharesActiveGroup(me, linked)`; jika tidak lagi berbagi → `ErrNotShared` 409 (user unlink kontak dulu).

**Approve** — `LoanRepository.Approve(ctx, id, approver, accountID) (toEntryID uuid.UUID, err)`, satu tx:
1. `SELECT status, to_entry_id, from_entry_id, from_user_id, amount FROM loan_requests WHERE id=$1 AND to_user_id=$2 FOR UPDATE` → no row: `ErrNotFound`. `approved` → return `to_entry_id` yang ada (idempoten, tidak posting ulang). `rejected` → `ErrLoanNotPending`.
2. Validasi `accounts WHERE id=$acc AND user_id=$approver` → `ErrAccountInvalid`.
3. Ambil contact line asal (`account code`, debit, credit). Mirror: contact line = `{counterpart(code), debit: asal.credit, credit: asal.debit}` dengan counterpart 1101↔2101 dicari di `accounts WHERE user_id=$approver AND code=$x`; other line = `{accountID, debit: asal.debit, credit: asal.credit}`. Kontak approver untuk A: `SELECT id FROM contacts WHERE user_id=$approver AND linked_user_id=$from` → tidak ada → insert (name = A.name).
4. Insert `journal_entries` (user_id=approver, kind sama, description `"Talangan dari <nama A>"` / `"Pembayaran dari <nama A>"`) + 2 lines. Nomor entry memakai advisory lock yang sama dengan Fase 1.
5. `UPDATE loan_requests SET status='approved', to_entry_id=$e, responded_at=now() WHERE id=$1 AND status='pending'`; rows=0 → rollback, `ErrLoanNotPending`. Commit.

`FOR UPDATE` membuat klik kedua menunggu commit pertama lalu membaca `approved` → jalur idempoten. UNIQUE(`to_entry_id`) sabuk pengaman kedua.

**Edit/delete entry asal (hook di journal service Fase 1):** Update → `SELECT 1 FROM loan_requests WHERE (from_entry_id=$1 OR to_entry_id=$1) AND status<>'rejected'` ada → `ErrEntryLocked` 409. Delete → dalam tx: `DELETE FROM loan_requests WHERE from_entry_id=$1 AND status<>'approved'` lalu delete entry; FK RESTRICT menolak jika approved → map pg code `23503` → 409. Reject menyisakan piutang A; list entry Fase 1 `LEFT JOIN loan_requests` untuk kolom `loan_status` (badge "Ditolak").

**fail():** `ErrNotFound`→404, `ErrLoanNotPending`/`ErrEntryLocked`/`ErrNotShared`→409, `ErrAccountInvalid`→400.

## Frontend
- Route `/app/groups` (`src/pages/groups.tsx`, satu file): bagian **Inbox** di atas (invite: Terima/Tolak; loan: `<select>` akun + Setujui/Tolak), lalu daftar grup: form buat grup, kartu member, daftar "Menunggu" berisi email + tombol Batalkan, form invite email, tombol Keluar, period picker Fase 3 + tabel ringkasan per member.
- `site.tsx` / app-shell: link "Grup" + `<Badge>` jumlah inbox.
- `api.ts`: types `Group`, `Inbox`; fungsi `listGroups, createGroup, inviteToGroup, respondInvite(inviteId,'accept'|'reject'), cancelInvite(groupId,inviteId), leaveGroup, groupSummary(id,date), approveLoan(id,account_id), rejectLoan(id)`; `useInbox()` — `useState` + `setInterval(30_000)` memanggil `authed('/inbox')`, berhenti saat `document.hidden`. Ponytail: polling 30s, bukan SSE.

## Test
`internal/handler/share_test.go` dengan fake in-memory (pola `fakeRepo`):
1. **IDOR**: C bukan member → `GET /groups/:id/summary` 404; B baru diundang (belum accept) → 404; setelah accept → 200; C tidak bisa accept invite milik email B → 404.
2. **Leave revokes**: B leave → B 404 dan A tidak lagi melihat B di `GET /groups`.
3. **Invite enumeration**: body & status identik untuk email terdaftar vs tidak terdaftar; keduanya muncul sama di `pending_invites`.
4. **Approve idempoten + mirror balance**: approve 2x → 200 keduanya, `to_entry_id` sama, fake punya tepat satu entry baru dengan Σdebit=Σcredit dan line 2101 contact=A.

`internal/repository/loan_integration_test.go` (`//go:build integration`, skip jika `TEST_DB_SOURCE` kosong): 10 goroutine approve request yang sama → tepat 1 baris `journal_entries` baru. Ini satu-satunya test yang membuktikan `FOR UPDATE`.

## Langkah kerja berurutan
1. Migrasi 000011, 000012; `migrate up`.
2. `model/group.go` (request structs + Normalize email), `repository/group.go` (Create, ListMine, Invite, CancelInvite, Accept, Reject, Leave, SharesActiveGroup, InvitesForEmail).
3. `repository/loan.go` (Approve tx, Reject, PendingForUser, IsEntryLocked) + statement insert di `JournalRepository.Create`.
4. `service/share.go` + interfaces; cek link kontak di contact service; hook lock di journal service.
5. `handler/share.go`, routes, `fail`, wiring `main.go`.
6. Test fakes + 4 test handler; integration test.
7. `api.ts`, `groups.tsx`, header badge, route.

## Konsep Go yang dipelajari (↔ Laravel)
- `db.BeginTx` / `defer tx.Rollback()` / `tx.Commit()` ↔ `DB::transaction()`; `SELECT … FOR UPDATE` ↔ `lockForUpdate()`.
- `INSERT … ON CONFLICT DO UPDATE` ↔ `upsert()`; `pgconn.PgError.Code == "23503"` ↔ `QueryException` FK.
- Error sentinel + `errors.Is` di `fail()` ↔ `Handler::render` mapping exception.
- Interface kecil dideklarasikan di consumer (service) ↔ contract binding, tanpa container.
- `//go:build integration` + `t.Skip` ↔ `RefreshDatabase` test yang hanya jalan di CI.
- Goroutine + `sync.WaitGroup` di integration test ↔ (tidak ada padanan langsung di PHP-FPM).

## Risiko & pertanyaan terbuka
- Undangan per email berarti user harus login dengan email yang sama persis (setelah lowercase). Ganti email belum didukung app, jadi aman untuk sekarang.
- Timing invite (email ada → extra INSERT) masih bisa diukur; mitigasi murah: rate limit per jam. Diterima untuk aplikasi pribadi.
- Approved entry terkunci permanen; koreksi via entry `adjustment` manual di kedua buku. Reverse otomatis = fase lanjutan jika dibutuhkan.
- Grup tanpa member active (semua leave) dibiarkan sebagai baris yatim; bersihkan nanti.
