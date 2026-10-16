package handler

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/mail"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/totp"
)

// fakeSessions is an in-memory service.SessionRepository with the same rules as the SQL one.
type fakeSessions struct {
	mu   sync.Mutex
	rows map[string]*fakeSession
	seq  int // stands in for created_at ordering
}

type fakeSession struct {
	userID, sessionID uuid.UUID
	ua                string
	expires           time.Time
	created           int
	revoked, ended    bool
}

func newFakeSessions() *fakeSessions { return &fakeSessions{rows: map[string]*fakeSession{}} }

func (f *fakeSessions) Create(_ context.Context, userID, sessionID uuid.UUID, hash []byte, expiresAt time.Time, ua string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.rows[string(hash)] = &fakeSession{userID: userID, sessionID: sessionID, ua: ua, expires: expiresAt, created: f.seq}
	return nil
}

func (f *fakeSessions) live(r *fakeSession) bool { return !r.revoked && time.Now().Before(r.expires) }

func (f *fakeSessions) Consume(_ context.Context, hash []byte) (uuid.UUID, uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[string(hash)]
	switch {
	case !ok, row.ended:
		return uuid.Nil, uuid.Nil, repository.ErrRefreshNotFound
	case row.revoked:
		return row.userID, uuid.Nil, repository.ErrRefreshReused
	case !f.live(row):
		return uuid.Nil, uuid.Nil, repository.ErrRefreshNotFound
	}
	row.revoked = true
	return row.userID, row.sessionID, nil
}

func (f *fakeSessions) End(_ context.Context, userID, sessionID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	for _, r := range f.rows {
		if r.userID == userID && r.sessionID == sessionID && !r.ended {
			r.revoked, r.ended, found = true, true, true
		}
	}
	if !found {
		return repository.ErrSessionNotFound
	}
	return nil
}

func (f *fakeSessions) RevokeAll(_ context.Context, userID, keep uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.userID == userID && r.sessionID != keep {
			r.revoked, r.ended = true, true
		}
	}
	return nil
}

func (f *fakeSessions) SessionOf(_ context.Context, hash []byte) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[string(hash)]; ok && f.live(r) {
		return r.sessionID, nil
	}
	return uuid.Nil, repository.ErrSessionNotFound
}

func (f *fakeSessions) List(_ context.Context, userID uuid.UUID) ([]model.SessionInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	newest := map[uuid.UUID]*fakeSession{}
	alive := map[uuid.UUID]bool{}
	for _, r := range f.rows {
		if r.userID != userID {
			continue
		}
		if n := newest[r.sessionID]; n == nil || r.created > n.created {
			newest[r.sessionID] = r
		}
		alive[r.sessionID] = alive[r.sessionID] || f.live(r)
	}
	out := []model.SessionInfo{}
	for id, r := range newest {
		if alive[id] {
			out = append(out, model.SessionInfo{ID: id, UserAgent: r.ua, LastActive: time.Unix(int64(r.created), 0)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActive.After(out[j].LastActive) })
	return out, nil
}

// fakeSecurity is an in-memory service.SecurityRepository.
type fakeSecurity struct {
	mu     sync.Mutex
	links  map[string]*fakeLink
	backup map[uuid.UUID]map[string]bool // code hash -> used
}

type fakeLink struct {
	userID  uuid.UUID
	purpose string
	expires time.Time
	used    bool
}

func newFakeSecurity() *fakeSecurity {
	return &fakeSecurity{links: map[string]*fakeLink{}, backup: map[uuid.UUID]map[string]bool{}}
}

func (f *fakeSecurity) CreateEmailToken(_ context.Context, userID uuid.UUID, purpose string, hash []byte, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.links {
		if l.userID == userID && l.purpose == purpose {
			l.used = true
		}
	}
	f.links[string(hash)] = &fakeLink{userID: userID, purpose: purpose, expires: expiresAt}
	return nil
}

func (f *fakeSecurity) UseEmailToken(_ context.Context, purpose string, hash []byte) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[string(hash)]
	if !ok || l.used || l.purpose != purpose || time.Now().After(l.expires) {
		return uuid.Nil, repository.ErrInvalidLink
	}
	l.used = true
	return l.userID, nil
}

func (f *fakeSecurity) ReplaceBackupCodes(_ context.Context, userID uuid.UUID, hashes [][]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	codes := map[string]bool{}
	for _, h := range hashes {
		codes[string(h)] = false
	}
	f.backup[userID] = codes
	return nil
}

func (f *fakeSecurity) UseBackupCode(_ context.Context, userID uuid.UUID, hash []byte) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	used, ok := f.backup[userID][string(hash)]
	if !ok || used {
		return false, nil
	}
	f.backup[userID][string(hash)] = true
	return true, nil
}

// fakeMail keeps sent messages; the service sends from a goroutine, so readers poll.
type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

var mailboxes sync.Map // *gin.Engine -> *fakeMail

// linkToken waits for the newest email to `to` that links to path and returns its token.
func linkToken(t *testing.T, r *gin.Engine, to, path string) string {
	t.Helper()
	box, _ := mailboxes.Load(r)
	for range 100 {
		m := box.(*fakeMail)
		m.mu.Lock()
		for i := len(m.sent) - 1; i >= 0; i-- {
			msg := m.sent[i]
			if _, after, ok := strings.Cut(msg.Text, "http://app.test"+path+"?token="); ok && msg.To == to {
				m.mu.Unlock()
				return strings.Fields(after)[0]
			}
		}
		m.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s email to %s", path, to)
	return ""
}

func mailCount(r *gin.Engine, to string) int {
	time.Sleep(50 * time.Millisecond) // let a stray goroutine finish
	box, _ := mailboxes.Load(r)
	m := box.(*fakeMail)
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, msg := range m.sent {
		if msg.To == to {
			n++
		}
	}
	return n
}

// browser sends a request as one device: a User-Agent and the refresh cookie it holds.
// It returns the status, body, and the new cookie value when the response set one.
func browser(t *testing.T, r *gin.Engine, method, path, tok, cookie, ua, body string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: refreshCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, cookieValue(w.Result().Cookies())
}

func TestForgotAndResetPassword(t *testing.T) {
	r, _ := newTestServer()
	call(t, r, "POST", "/api/v1/register", "", `{"name":"Budi","email":"budi@x.com","password":"secret123"}`)

	code, unknown := call(t, r, "POST", "/api/v1/password/forgot", "", `{"email":"nobody@x.com"}`)
	expect(t, "forgot unknown email", code, 202, unknown)
	code, known := call(t, r, "POST", "/api/v1/password/forgot", "", `{"email":"Budi@X.com"}`)
	expect(t, "forgot known email", code, 202, known)
	if unknown["message"] != known["message"] {
		t.Fatal("known and unknown emails must get the same answer")
	}
	if mailCount(r, "nobody@x.com") != 0 {
		t.Fatal("no email may go to an unregistered address")
	}

	// A link asked for earlier stops working when a newer one is sent.
	old := linkToken(t, r, "budi@x.com", "/reset-password")
	call(t, r, "POST", "/api/v1/password/forgot", "", `{"email":"budi@x.com"}`)
	var tok string
	for tok = old; tok == old; {
		tok = linkToken(t, r, "budi@x.com", "/reset-password")
	}
	code, body := call(t, r, "POST", "/api/v1/password/reset", "", `{"token":"`+old+`","new_password":"newpass123"}`)
	expect(t, "older link", code, 400, body)

	_, _, cookie := browser(t, r, "POST", "/api/v1/login", "", "", "Phone", `{"email":"budi@x.com","password":"secret123"}`)
	code, body = call(t, r, "POST", "/api/v1/password/reset", "", `{"token":"`+tok+`","new_password":"newpass123"}`)
	expect(t, "reset", code, 204, body)
	code, body = call(t, r, "POST", "/api/v1/password/reset", "", `{"token":"`+tok+`","new_password":"again12345"}`)
	if code != 400 || body["code"] != "invalid_link" {
		t.Fatalf("a used link must fail with invalid_link, got %d %v", code, body)
	}
	code, body, _ = refresh(t, r, cookie)
	expect(t, "session from before the reset", code, 401, body)

	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"secret123"}`)
	expect(t, "old password", code, 401, body)
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"newpass123"}`)
	expect(t, "new password", code, 200, body)
	if body["user"].(map[string]any)["email_verified"] != true {
		t.Fatal("opening a reset link proves the inbox, so the email counts as verified")
	}
}

func TestVerifyEmailGatesInvites(t *testing.T) {
	r, _ := newTestServer()
	lida := signUp(t, r, "lida@x.com")
	_, body := call(t, r, "POST", "/api/v1/groups", lida, `{"name":"Rumah"}`)
	gid := body["data"].(map[string]any)["id"].(string)
	call(t, r, "POST", "/api/v1/groups/"+gid+"/invites", lida, `{"email":"fikri@x.com"}`)

	call(t, r, "POST", "/api/v1/register", "", `{"name":"Fikri","email":"fikri@x.com","password":"secret123"}`)
	_, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"fikri@x.com","password":"secret123"}`)
	fikri := body["access_token"].(string)
	if body["user"].(map[string]any)["email_verified"] != false {
		t.Fatal("a new account starts unverified")
	}

	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	if n := len(body["data"].(map[string]any)["invites"].([]any)); n != 0 {
		t.Fatalf("an unverified account must not see invites to its email, saw %d", n)
	}

	code, body := call(t, r, "POST", "/api/v1/me/email/verification", fikri, "")
	expect(t, "resend verification", code, 202, body)
	for mailCount(r, "fikri@x.com") < 2 { // the resent link replaces the one from sign-up
	}
	tok := linkToken(t, r, "fikri@x.com", "/verify-email")
	code, body = call(t, r, "POST", "/api/v1/email/verify", "", `{"token":"`+tok+`"}`)
	expect(t, "verify", code, 204, body)
	code, body = call(t, r, "POST", "/api/v1/email/verify", "", `{"token":"`+tok+`"}`)
	expect(t, "verify twice", code, 400, body)

	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	if n := len(body["data"].(map[string]any)["invites"].([]any)); n != 1 {
		t.Fatalf("after verifying the invite shows, saw %d", n)
	}
}

func TestSessions(t *testing.T) {
	r, _ := newTestServer()
	call(t, r, "POST", "/api/v1/register", "", `{"name":"Budi","email":"budi@x.com","password":"secret123"}`)
	login := `{"email":"budi@x.com","password":"secret123"}`
	_, a, phone := browser(t, r, "POST", "/api/v1/login", "", "", "Phone", login)
	_, _, laptop := browser(t, r, "POST", "/api/v1/login", "", "", "Laptop", login)
	tokA := a["access_token"].(string)

	// Refreshing keeps the same session: still two devices.
	_, _, phone = browser(t, r, "POST", "/api/v1/auth/refresh", "", phone, "Phone", "")
	code, body, _ := browser(t, r, "GET", "/api/v1/auth/sessions", tokA, phone, "Phone", "")
	expect(t, "list sessions", code, 200, body)
	list := body["data"].([]any)
	if len(list) != 2 {
		t.Fatalf("want 2 sessions, got %v", list)
	}
	var laptopID string
	for _, s := range list {
		s := s.(map[string]any)
		if s["user_agent"] == "Laptop" {
			laptopID = s["id"].(string)
			if s["current"] != false {
				t.Fatal("the laptop is not the current device")
			}
		} else if s["current"] != true {
			t.Fatal("the phone is the current device")
		}
	}

	code, body, _ = browser(t, r, "DELETE", "/api/v1/auth/sessions/"+laptopID, tokA, phone, "Phone", "")
	expect(t, "end laptop session", code, 204, body)
	code, body, _ = refresh(t, r, laptop)
	expect(t, "ended session refreshes", code, 401, body)
	// An ended session coming back is not token theft: the phone keeps working.
	code, body, phone = browser(t, r, "POST", "/api/v1/auth/refresh", "", phone, "Phone", "")
	expect(t, "phone still signed in", code, 200, body)
	tokA = body["access_token"].(string)

	_, _, tablet := browser(t, r, "POST", "/api/v1/login", "", "", "Tablet", login)
	code, body, _ = browser(t, r, "POST", "/api/v1/auth/sessions/revoke-others", tokA, phone, "Phone", "")
	expect(t, "revoke others", code, 200, body)
	fresh := body["access_token"].(string)
	code, body, _ = refresh(t, r, tablet)
	expect(t, "tablet after revoke-others", code, 401, body)
	code, body = call(t, r, "GET", "/api/v1/me", tokA, "")
	expect(t, "access token from before revoke-others", code, 401, body)
	code, body = call(t, r, "GET", "/api/v1/me", fresh, "")
	expect(t, "new access token", code, 200, body)
}

func TestTOTP(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "budi@x.com")

	code, body := call(t, r, "POST", "/api/v1/me/totp/enable", tok, `{"code":"123456"}`)
	if code != 409 || body["code"] != "totp_not_setup" {
		t.Fatalf("enable before setup: %d %v", code, body)
	}
	code, body = call(t, r, "POST", "/api/v1/me/totp/setup", tok, "")
	expect(t, "setup", code, 200, body)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(body["data"].(map[string]any)["secret"].(string))
	if err != nil {
		t.Fatal(err)
	}
	step := time.Now().Unix() / 30
	codeAt := func(s int64) string { return totp.Code(secret, s) }

	// Setup alone does not change login.
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"secret123"}`)
	if code != 200 || body["access_token"] == nil {
		t.Fatalf("login before enabling: %d %v", code, body)
	}

	wrong := "000000"
	if codeAt(step) == wrong {
		wrong = "000001"
	}
	code, body = call(t, r, "POST", "/api/v1/me/totp/enable", tok, `{"code":"`+wrong+`"}`)
	expect(t, "enable with a wrong code", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/me/totp/enable", tok, `{"code":"`+codeAt(step)+`"}`)
	expect(t, "enable", code, 200, body)
	backup := body["backup_codes"].([]any)
	if len(backup) != 10 {
		t.Fatalf("want 10 backup codes, got %v", backup)
	}

	login := func() string {
		code, body, cookie := browser(t, r, "POST", "/api/v1/login", "", "", "Phone", `{"email":"budi@x.com","password":"secret123"}`)
		if code != 200 || body["mfa_required"] != true || body["access_token"] != nil || cookie != "" {
			t.Fatalf("login with 2FA must stop at the code step: %d %v cookie=%q", code, body, cookie)
		}
		return body["mfa_token"].(string)
	}
	ticket := login()
	code, body = call(t, r, "GET", "/api/v1/me", ticket, "")
	expect(t, "mfa ticket used as access token", code, 401, body)

	second := func(code string) (int, map[string]any) {
		return call(t, r, "POST", "/api/v1/login/totp", "", `{"mfa_token":"`+ticket+`","code":"`+code+`"}`)
	}
	code, body = second(codeAt(step))
	if code != 400 || body["code"] != "invalid_code" {
		t.Fatalf("the code that enabled 2FA must not log in again: %d %v", code, body)
	}
	code, body = second(codeAt(step + 1)) // next step, within the drift window
	expect(t, "login with the next code", code, 200, body)

	ticket = login()
	code, body = second(strings.ToUpper(backup[0].(string)))
	expect(t, "login with a backup code", code, 200, body)
	ticket = login()
	code, body = second(backup[0].(string))
	expect(t, "backup code twice", code, 400, body)

	code, body = call(t, r, "POST", "/api/v1/me/totp/disable", tok, `{"password":"wrong","code":"`+backup[1].(string)+`"}`)
	expect(t, "disable with a wrong password", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/me/totp/disable", tok, `{"password":"secret123","code":"`+backup[1].(string)+`"}`)
	expect(t, "disable", code, 204, body)
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"secret123"}`)
	if code != 200 || body["access_token"] == nil {
		t.Fatalf("login after disabling 2FA: %d %v", code, body)
	}
}

// ExportTables is covered against Postgres; the fake only keeps the interface satisfied.
func (f *fakeLedger) ExportTables(context.Context, uuid.UUID) ([]model.ExportTable, error) {
	return []model.ExportTable{{Name: "accounts", Columns: []string{"code"}, Rows: [][]string{}}}, nil
}
