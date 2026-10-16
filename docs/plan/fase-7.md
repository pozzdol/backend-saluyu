# Fase 7 — Keamanan Akun dan Onboarding Ramah

Menjawab masukan A26, A27, B5, B21, B22 (lihat `masukan-dan-fase-lanjutan.md`).

## Tujuan & definisi selesai
1. **Lupa password:** user meminta link reset lewat email. Link berlaku 30 menit dan hanya sekali pakai. Setelah reset, semua sesi dicabut. Respons permintaan selalu sama, terdaftar atau tidak.
2. **Verifikasi email:** email dikirim saat daftar dan bisa dikirim ulang dari Profil. Undangan grup hanya terlihat dan bisa diterima oleh akun yang emailnya sudah terverifikasi, karena undangan dicocokkan lewat email.
3. **Sesi aktif:** Profil menampilkan perangkat (dari User-Agent), mulai login, dan terakhir aktif. Ada tombol cabut per sesi dan "keluar dari semua perangkat lain".
4. **2FA TOTP (opsional):** aktif lewat aplikasi authenticator (QR + kode manual), dengan 10 kode cadangan sekali pakai. Login meminta kode setelah password.
5. **Wizard onboarding** untuk user yang belum membuka buku: pilih bank/e-wallet → isi saldo → tanggal gajian → (opsional) 3 budget pertama. Tidak ada kata "aset" atau "liabilitas" di dalamnya.
6. **Privasi & Data:** halaman yang menjelaskan siapa melihat apa di grup, plus unduh semua data (ZIP: `data.json` dan CSV per tabel).
7. Test hijau, plus uji manual ke Postgres sungguhan dan browser (desktop + 390px).

## Di luar cakupan
Ganti email, login sosial (Google), WebAuthn/passkey, notifikasi email selain reset/verifikasi, hapus akun (Fase 11).

## Database
**`000014_add_account_security.up.sql`**
```sql
ALTER TABLE users
  ADD COLUMN email_verified_at   TIMESTAMPTZ,
  ADD COLUMN sessions_revoked_at TIMESTAMPTZ,  -- "log out everywhere": older access tokens stop working
  ADD COLUMN totp_secret         BYTEA,        -- AES-GCM encrypted; NULL = no 2FA set up
  ADD COLUMN totp_enabled_at     TIMESTAMPTZ,  -- NULL while setup is not confirmed with a code
  ADD COLUMN totp_last_step      BIGINT NOT NULL DEFAULT 0; -- a code works once (no replay)
-- Accounts made before verification existed count as verified.
UPDATE users SET email_verified_at = created_at;

-- A session is a chain of rotated refresh tokens that share session_id.
ALTER TABLE refresh_tokens
  ADD COLUMN session_id UUID NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN user_agent TEXT NOT NULL DEFAULT '',
  -- ended = revoked on purpose (logout, revoke). A revoked but not ended token that comes
  -- back was rotated, so someone copied it: that still ends every session.
  ADD COLUMN ended BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX refresh_tokens_session_idx ON refresh_tokens (session_id);

CREATE TABLE email_tokens (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose    VARCHAR(10) NOT NULL CHECK (purpose IN ('verify', 'reset')),
  token_hash BYTEA NOT NULL UNIQUE, -- SHA-256 of the link token
  expires_at TIMESTAMPTZ NOT NULL,
  used_at    TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE totp_backup_codes (
  id        BIGSERIAL PRIMARY KEY,
  user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash BYTEA NOT NULL, -- SHA-256; codes have 50 random bits
  used_at   TIMESTAMPTZ
);
CREATE INDEX totp_backup_codes_user_idx ON totp_backup_codes (user_id);
```

## Backend
### Email (`internal/mail`)
- `Sender` dengan dua implementasi: **Resend** (`POST https://api.resend.com/emails`, `Authorization: Bearer RESEND_API_KEY`) dan **Log** (mencetak isi email ke log server; dipakai bila `RESEND_API_KEY` kosong).
- Config baru: `RESEND_API_KEY` (opsional), `MAIL_FROM` (default `Saluyu <noreply@fikriachmad.dev>`), `APP_URL` (default `http://localhost:5173`, dasar link di email).
- Email berupa teks biasa (tanpa HTML). Dikirim di goroutine dengan timeout 10 detik, supaya waktu respons tidak membocorkan apakah email terdaftar.
- Bahasa email: `users.locale`, kalau kosong mengikuti `Accept-Language` request.

### Endpoint
| Method | Path | Isi |
|---|---|---|
| POST | `/password/forgot` | `{email}` → selalu 202. Token reset 30 menit; token reset lama milik user itu ikut hangus. |
| POST | `/password/reset` | `{token, new_password}` → ganti password, cabut semua sesi, tandai email terverifikasi (link membuktikan kepemilikan). |
| POST | `/email/verify` | `{token}` → set `email_verified_at`. Token berlaku 48 jam. |
| POST | `/me/email/verification` | kirim ulang link verifikasi (dibatasi 3/menit). |
| POST | `/login` | bila 2FA aktif: `200 {mfa_required: true, mfa_token}` (JWT 5 menit dengan `purpose: "mfa"`). |
| POST | `/login/totp` | `{mfa_token, code}` → sesi. `code` = 6 digit TOTP atau kode cadangan. |
| GET | `/auth/sessions` | daftar sesi hidup; `current: true` untuk sesi cookie ini. |
| DELETE | `/auth/sessions/:id` | cabut satu sesi. |
| POST | `/auth/sessions/revoke-others` | cabut semua kecuali sesi ini, dan set `sessions_revoked_at`. |
| POST | `/me/totp/setup` | buat secret baru (belum aktif) → `{secret, uri}`. |
| POST | `/me/totp/enable` | `{code}` → aktifkan, kembalikan 10 kode cadangan (hanya sekali). |
| POST | `/me/totp/disable` | `{password, code}` → matikan, hapus kode cadangan. |
| GET | `/me/export` | ZIP: `data.json`, `accounts.csv`, `journal.csv`, `contacts.csv`, `budgets.csv`. |

`UserResponse` mendapat `email_verified` dan `totp_enabled`. Route sesi ada di bawah `/auth` karena cookie refresh hanya dikirim ke path itu, dan sesi yang sedang dipakai dikenali dari cookie.

### Aturan
- **TOTP** (RFC 6238, SHA-1, 30 detik, 6 digit, toleransi ±1 langkah) ditulis dengan stdlib (`crypto/hmac`, `encoding/base32`). Langkah yang sudah dipakai disimpan di `totp_last_step`, jadi kode yang sama tidak bisa dipakai dua kali.
- **Secret TOTP dienkripsi** AES-256-GCM dengan kunci `SHA-256("totp:" + JWT_SECRET)`. Bocornya tabel saja tidak cukup untuk membuat kode.
- **Token akses berjenis `mfa`** ditolak oleh middleware `Auth`, sehingga tidak bisa dipakai sebagai token akses.
- `Me` menolak token yang `iat`-nya sebelum `max(password_changed_at, sessions_revoked_at)`.
- **Batas laju:** `/password/forgot` 5/menit, `/password/reset` 10/menit, `/email/verify` 10/menit, `/login/totp` 10/menit (per IP).
- **Undangan grup:** `InvitesForEmail` dan menerima undangan mensyaratkan `email_verified_at IS NOT NULL`.

### Error baru
`invalid_link` (400: token reset/verifikasi salah, kedaluwarsa, atau sudah dipakai), `mfa_expired` (401 untuk `mfa_token` tidak sah atau kedaluwarsa), `invalid_code` (400/401: kode TOTP salah), `totp_not_setup` (409), `totp_already_enabled` (409), `email_unverified` (403), `session_not_found` (404).

## Frontend
- **Login:** setelah `mfa_required`, tampil langkah kedua "Kode dari aplikasi authenticator", dengan tautan "Pakai kode cadangan".
- **Lupa password:** `/forgot-password` (form email → pesan yang sama apa pun hasilnya) dan `/reset-password?token=…` (password baru + konfirmasi).
- **Verifikasi:** `/verify-email?token=…`. Banner di app shell selama email belum terverifikasi, dengan tombol kirim ulang. Groups menjelaskan kenapa undangan belum terlihat.
- **Profil → Keamanan:** daftar sesi (perangkat, terakhir aktif, "Perangkat ini"), cabut per sesi, keluar dari semua perangkat lain; aktifkan/nonaktifkan 2FA (QR dari package `uqr`, secret untuk diketik manual, lalu kode cadangan yang bisa disalin/diunduh).
- **Wizard `/app/welcome`** (user tanpa `start_date` diarahkan ke sini, bukan ke Saldo Awal):
  1. *Uangmu ada di mana?* Kotak pilihan BCA, Mandiri, BRI, BNI, Jago, SeaBank, blu, GoPay, OVO, DANA, ShopeePay, dan Tunai (selalu ada). Bank pertama mengganti nama akun 1001 "Bank", e-wallet pertama mengganti nama 1004 "E-wallet"; pilihan berikutnya menjadi akun baru.
  2. *Berapa isinya hari ini?* Satu field jumlah per pilihan, plus "Utang yang sedang berjalan?" (opsional, akun 2xxx yang sudah ada).
  3. *Kapan kamu gajian?* Pilih tanggal 1–28.
  4. *Mau batasi pengeluaran?* (opsional) 3 kategori teratas dengan jumlah.
  5. Simpan: rename/buat akun → `PUT /opening` → `PATCH /me` → budget → ke Ringkasan.
  Ikon bank berupa inisial berwarna, bukan logo resmi (hak merek).
- **Privasi & Data** (`/app/privacy`, link di menu Profil): siapa melihat apa (anggota grup hanya total periode; talangan hanya terlihat oleh kontak yang ditautkan), lalu tombol unduh ZIP.

## Test
- `security_test.go` (handler, fake in-memory): forgot selalu 202 dan tidak mengirim apa pun untuk email asing; reset sekali pakai, kedaluwarsa, dan mencabut sesi; verifikasi; undangan tersembunyi sebelum terverifikasi; sesi: daftar, cabut satu, cabut lainnya, token yang dicabut tidak memicu revoke-all; TOTP: setup → enable → login dua langkah, kode dipakai ulang ditolak, kode cadangan sekali pakai, `mfa_token` ditolak sebagai token akses.
- `totp_test.go`: vektor uji RFC 6238 (SHA-1) dan toleransi ±1 langkah.
- `export_test.go`: ZIP berisi semua file, CSV journal seimbang.
- Frontend: kunci i18n tetap sama (test yang ada).

## Langkah kerja berurutan
1. Migration 000014 + model/repository.
2. `internal/mail` + config.
3. Lupa password + verifikasi email + gerbang undangan.
4. Sesi (session_id, user agent, daftar/cabut).
5. TOTP + kode cadangan + login dua langkah.
6. Ekspor data.
7. Test backend, smoke test Postgres.
8. Frontend: halaman auth baru, banner verifikasi, Profil → Keamanan, 2FA, wizard, Privasi & Data, i18n.
9. Verifikasi browser.

## Risiko & catatan
- **Domain Resend** `fikriachmad.dev` perlu diverifikasi (record SPF/DKIM di DNS) sebelum email benar-benar terkirim. Sampai itu, mode Log mencetak link di log server.
- Mencabut satu sesi menghentikan refresh-nya, tetapi token akses yang sudah terbit tetap berlaku sampai kedaluwarsa (paling lama `JWT_DURATION`, 15 menit). "Keluar dari semua perangkat lain" langsung menolak token lama di `/me`.
- Route ledger tidak memeriksa `password_changed_at` per request (sudah begitu sejak Fase 1); batasnya sama, 15 menit.
- Rate limit masih in-memory per proses.
