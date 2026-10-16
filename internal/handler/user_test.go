package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

// fakeRepo is an in-memory service.UserRepository with the same unique-email rule as the DB.
type fakeRepo struct {
	mu     sync.Mutex
	users  map[uuid.UUID]*model.User
	ledger *fakeLedger // start_date lives on users in the real DB; the fake ledger owns it here
}

func (f *fakeRepo) Create(_ context.Context, u *model.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, other := range f.users {
		if strings.EqualFold(other.Email, u.Email) {
			return repository.ErrEmailTaken
		}
	}
	u.ID, u.CreatedAt, u.UpdatedAt = uuid.New(), time.Now(), time.Now()
	u.PeriodStartDay = 25 // column default
	copied := *u
	f.users[u.ID] = &copied
	return nil
}

func (f *fakeRepo) FindByEmail(_ context.Context, email string) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Email == email {
			copied := *u
			return &copied, nil
		}
	}
	return nil, repository.ErrUserNotFound
}

func (f *fakeRepo) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	copied := *u
	if f.ledger != nil {
		copied.StartDate, _ = f.ledger.StartDate(context.Background(), id)
	}
	return &copied, nil
}

func (f *fakeRepo) Update(_ context.Context, id uuid.UUID, req model.UpdateProfileRequest) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	if req.Name != nil {
		u.Name = *req.Name
	}
	if req.Image != nil {
		u.Image = *req.Image
	}
	if req.PeriodStartDay != nil {
		u.PeriodStartDay = *req.PeriodStartDay
		if f.ledger != nil {
			f.ledger.setPeriodStartDay(id, u.PeriodStartDay)
		}
	}
	u.UpdatedAt = time.Now()
	copied := *u
	return &copied, nil
}

func (f *fakeRepo) UpdatePassword(_ context.Context, id uuid.UUID, hash string, changedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return repository.ErrUserNotFound
	}
	u.Password, u.PasswordChangedAt = hash, changedAt
	return nil
}

// fakeSessions is an in-memory service.SessionRepository with the same rules as the SQL one.
type fakeSessions struct {
	mu   sync.Mutex
	rows map[string]*fakeSession
}

type fakeSession struct {
	userID  uuid.UUID
	expires time.Time
	revoked bool
}

func (f *fakeSessions) Create(_ context.Context, userID uuid.UUID, hash []byte, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[string(hash)] = &fakeSession{userID: userID, expires: expiresAt}
	return nil
}

func (f *fakeSessions) Consume(_ context.Context, hash []byte) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[string(hash)]
	switch {
	case !ok:
		return uuid.Nil, repository.ErrRefreshNotFound
	case row.revoked:
		return row.userID, repository.ErrRefreshReused
	case time.Now().After(row.expires):
		return uuid.Nil, repository.ErrRefreshNotFound
	}
	row.revoked = true
	return row.userID, nil
}

func (f *fakeSessions) RevokeAll(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if row.userID == userID {
			row.revoked = true
		}
	}
	return nil
}

func newTestServer() (*gin.Engine, *fakeRepo) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{users: map[uuid.UUID]*model.User{}}
	sessions := &fakeSessions{rows: map[string]*fakeSession{}}
	tm := token.NewMaker("test-secret", time.Minute)
	r := gin.New()
	fl := newFakeLedger()
	repo.ledger = fl
	ledger := NewLedgerHandler(service.NewLedgerService(fl))
	reports := NewReportHandler(service.NewReportService(fl))
	RegisterRoutes(r, NewUserHandler(service.NewUserService(repo, sessions, tm, time.Hour)), ledger, reports, tm)
	return r, repo
}

// refresh posts to /auth/refresh with the given cookie value and returns the status,
// body and the rotated cookie value ("" when none was set).
func refresh(t *testing.T, r *gin.Engine, cookie string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookie, Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, cookieValue(w.Result().Cookies())
}

func cookieValue(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == refreshCookie {
			return c.Value
		}
	}
	return ""
}

// call sends a JSON request and returns the status code and decoded body.
func call(t *testing.T, r *gin.Engine, method, path, tok, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func expect(t *testing.T, step string, got, want int, body map[string]any) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: status %d, want %d, body %v", step, got, want, body)
	}
}

func TestUserFlow(t *testing.T) {
	r, repo := newTestServer()

	code, body := call(t, r, "POST", "/api/v1/register", "", `{"name":"  Budi  ","email":" Budi@X.com ","password":"secret123"}`)
	expect(t, "register", code, 201, body)
	data := body["data"].(map[string]any)
	if data["name"] != "Budi" || data["email"] != "budi@x.com" {
		t.Fatalf("register did not normalize input: %v", data)
	}
	if _, leaked := data["password"]; leaked {
		t.Fatal("register response contains password")
	}

	code, body = call(t, r, "POST", "/api/v1/register", "", `{"name":"Budi","email":"BUDI@x.com","password":"secret123"}`)
	expect(t, "duplicate email with other case", code, 409, body)

	code, body = call(t, r, "POST", "/api/v1/register", "", `{"name":"   ","email":"a@x.com","password":"secret123"}`)
	expect(t, "blank name", code, 400, body)
	if body["fields"].(map[string]any)["name"] == nil {
		t.Fatalf("blank name: want error on field name, got %v", body)
	}

	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"wrong"}`)
	expect(t, "wrong password", code, 401, body)
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"nobody@x.com","password":"wrong"}`)
	expect(t, "unknown email", code, 401, body)

	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"BUDI@X.COM","password":"secret123"}`)
	expect(t, "login", code, 200, body)
	tok := body["access_token"].(string)

	code, body = call(t, r, "GET", "/api/v1/me", "", "")
	expect(t, "me without token", code, 401, body)
	code, body = call(t, r, "GET", "/api/v1/me", "garbage", "")
	expect(t, "me with bad token", code, 401, body)
	code, body = call(t, r, "GET", "/api/v1/me", tok, "")
	expect(t, "me", code, 200, body)

	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"img_url":"https://cdn.x.com/a.png"}`)
	expect(t, "update image only", code, 200, body)
	data = body["data"].(map[string]any)
	if data["name"] != "Budi" || data["img_url"] != "https://cdn.x.com/a.png" {
		t.Fatalf("partial update changed wrong fields: %v", data)
	}
	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"name":"  "}`)
	expect(t, "update blank name", code, 400, body)
	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"img_url":"not a url"}`)
	expect(t, "update bad url", code, 400, body)

	code, body = call(t, r, "PUT", "/api/v1/me/password", tok, `{"old_password":"wrong","new_password":"newsecret"}`)
	expect(t, "change password wrong old", code, 400, body)
	code, body = call(t, r, "PUT", "/api/v1/me/password", tok, `{"old_password":"secret123","new_password":"newsecret"}`)
	expect(t, "change password", code, 200, body)
	newTok, _ := body["access_token"].(string)
	code, body = call(t, r, "GET", "/api/v1/me", tok, "")
	expect(t, "me with token from before password change", code, 401, body)
	code, body = call(t, r, "GET", "/api/v1/me", newTok, "")
	expect(t, "me with token returned by password change", code, 200, body)
	tok = newTok
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"secret123"}`)
	expect(t, "login with old password", code, 401, body)
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"newsecret"}`)
	expect(t, "login with new password", code, 200, body)

	for _, u := range repo.users {
		u.IsActive = false
	}
	code, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"budi@x.com","password":"newsecret"}`)
	expect(t, "login inactive", code, 403, body)
	code, body = call(t, r, "GET", "/api/v1/me", tok, "")
	expect(t, "me inactive with old token", code, 403, body)
}

func TestPasswordOver72Bytes(t *testing.T) {
	r, _ := newTestServer()
	pw := strings.Repeat("é", 72) // 72 characters pass validation, but 144 bytes is too long for bcrypt
	code, body := call(t, r, "POST", "/api/v1/register", "", `{"name":"Budi","email":"b@x.com","password":"`+pw+`"}`)
	expect(t, "register long multibyte password", code, 400, body)
}

func TestLoginRateLimit(t *testing.T) {
	r, _ := newTestServer()
	for i := 1; i <= 10; i++ {
		code, body := call(t, r, "POST", "/api/v1/login", "", `{bad`)
		expect(t, "request under limit", code, 400, body)
	}
	code, body := call(t, r, "POST", "/api/v1/login", "", `{bad`)
	expect(t, "request over limit", code, 429, body)
}

func TestRefreshRotationAndLogout(t *testing.T) {
	r, _ := newTestServer()
	call(t, r, "POST", "/api/v1/register", "", `{"name":"Budi","email":"b@x.com","password":"secret123"}`)

	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"email":"b@x.com","password":"secret123"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	expect(t, "login", w.Code, 200, nil)
	first := w.Result().Cookies()[0]
	if !first.HttpOnly || !first.Secure || first.SameSite != http.SameSiteStrictMode || first.Path != "/api/v1/auth" {
		t.Fatalf("refresh cookie is not locked down: %+v", first)
	}
	if strings.Contains(w.Body.String(), first.Value) {
		t.Fatal("refresh token leaked into the JSON body")
	}

	code, body, second := refresh(t, r, first.Value)
	expect(t, "refresh", code, 200, body)
	if second == "" || second == first.Value {
		t.Fatalf("refresh did not rotate the cookie")
	}
	code, body = call(t, r, "GET", "/api/v1/me", body["access_token"].(string), "")
	expect(t, "me with refreshed access token", code, 200, body)

	code, body, _ = refresh(t, r, first.Value)
	expect(t, "reuse rotated token", code, 401, body)
	code, body, _ = refresh(t, r, second)
	expect(t, "newest token after reuse was detected", code, 401, body)

	code, body, _ = refresh(t, r, "garbage")
	expect(t, "unknown token", code, 401, body)

	// Fresh login, then logout must kill that refresh token.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"email":"b@x.com","password":"secret123"}`)))
	third := cookieValue(w.Result().Cookies())
	req = httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookie, Value: third})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	expect(t, "logout", w.Code, 204, nil)
	code, body, _ = refresh(t, r, third)
	expect(t, "refresh after logout", code, 401, body)
}
