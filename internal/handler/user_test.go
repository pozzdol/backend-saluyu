package handler

import (
	"context"
	"encoding/json"
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
	mu    sync.Mutex
	users map[uuid.UUID]*model.User
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
	return &copied, nil
}

func (f *fakeRepo) Update(_ context.Context, id uuid.UUID, name, image *string) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	if name != nil {
		u.Name = *name
	}
	if image != nil {
		u.Image = *image
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

func newTestServer() (*gin.Engine, *fakeRepo) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{users: map[uuid.UUID]*model.User{}}
	tm := token.NewMaker("test-secret", time.Minute)
	r := gin.New()
	RegisterRoutes(r, NewUserHandler(service.NewUserService(repo, tm)), tm)
	return r, repo
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
	tok := body["token"].(string)

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
	newTok, _ := body["token"].(string)
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
