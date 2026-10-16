package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/token"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken         = repository.ErrEmailTaken
	ErrUserNotFound       = repository.ErrUserNotFound
	ErrInvalidCredentials = errors.New("password or email not match")
	ErrUserInactive       = errors.New("user is inactive")
	ErrWrongPassword      = errors.New("old password is incorrect")
	ErrTokenRevoked       = errors.New("token revoked, please log in again")
	ErrSessionExpired     = errors.New("session expired, please log in again")
	// validation counts characters, bcrypt counts bytes: 72 multi-byte characters can still be too long.
	ErrPasswordTooLong = bcrypt.ErrPasswordTooLong
)

// dummyHash is compared against when the email does not exist, so a login for an
// unknown email takes as long as one with a wrong password (no account enumeration by timing).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

// UserRepository is what the service needs from storage.
// *repository.UserRepository implements it; tests use an in-memory fake.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	Update(ctx context.Context, id uuid.UUID, name, image *string) (*model.User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, hash string, changedAt time.Time) error
}

// SessionRepository stores refresh tokens. *repository.SessionRepository implements it.
type SessionRepository interface {
	Create(ctx context.Context, userID uuid.UUID, hash []byte, expiresAt time.Time) error
	Consume(ctx context.Context, hash []byte) (uuid.UUID, error)
	RevokeAll(ctx context.Context, userID uuid.UUID) error
}

type UserService struct {
	repo       UserRepository
	sessions   SessionRepository
	token      *token.Maker
	refreshTTL time.Duration
}

func NewUserService(repo UserRepository, sessions SessionRepository, tm *token.Maker, refreshTTL time.Duration) *UserService {
	return &UserService{repo: repo, sessions: sessions, token: tm, refreshTTL: refreshTTL}
}

func (s *UserService) Register(ctx context.Context, req model.RegisterRequest) (*model.User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Name:     req.Name,
		Email:    req.Email,
		Password: string(hashed),
		IsActive: true,
		// Set here, not by the DB default: token iat comes from this clock too.
		PasswordChangedAt: time.Now(),
	}

	// ponytail: no pre-check, unique constraint in DB is the source of truth
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) Login(ctx context.Context, req model.LoginRequest) (*model.Session, error) {
	user, err := s.repo.FindByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Checked after the password so an attacker cannot probe which accounts are disabled.
	if !user.IsActive {
		return nil, ErrUserInactive
	}

	return s.startSession(ctx, user)
}

// Refresh trades a refresh token for a new access token and a new refresh token (rotation).
func (s *UserService) Refresh(ctx context.Context, refreshToken string) (*model.Session, error) {
	userID, err := s.sessions.Consume(ctx, hashToken(refreshToken))
	switch {
	case errors.Is(err, repository.ErrRefreshReused):
		// A rotated token came back, so someone else holds a copy. End every session of this user.
		if err := s.sessions.RevokeAll(ctx, userID); err != nil {
			return nil, err
		}
		return nil, ErrSessionExpired
	case errors.Is(err, repository.ErrRefreshNotFound):
		return nil, ErrSessionExpired
	case err != nil:
		return nil, err
	}

	user, err := s.repo.FindByID(ctx, userID)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrSessionExpired
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrUserInactive
	}
	return s.startSession(ctx, user)
}

// Logout revokes the refresh token. An unknown or already revoked token is not an error.
func (s *UserService) Logout(ctx context.Context, refreshToken string) error {
	_, err := s.sessions.Consume(ctx, hashToken(refreshToken))
	if errors.Is(err, repository.ErrRefreshNotFound) || errors.Is(err, repository.ErrRefreshReused) {
		return nil
	}
	return err
}

func (s *UserService) startSession(ctx context.Context, user *model.User) (*model.Session, error) {
	access, err := s.token.Create(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	refresh := rand.Text() // 128 random bits
	expires := time.Now().Add(s.refreshTTL)
	if err := s.sessions.Create(ctx, user.ID, hashToken(refresh), expires); err != nil {
		return nil, err
	}

	return &model.Session{
		AccessToken:    access,
		User:           model.ToUserResponse(user),
		RefreshToken:   refresh,
		RefreshExpires: expires,
	}, nil
}

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

// Me returns the logged-in user. A token stays valid until it expires, so every request
// re-checks the user: disabled after login, or token issued before the last password change.
func (s *UserService) Me(ctx context.Context, claims *token.Claims) (*model.User, error) {
	user, err := s.repo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrUserInactive
	}
	// ponytail: a token issued in the same millisecond as the change survives; not worth a token version column.
	// +1ms: iat travels as a float of seconds and the JWT library truncates it, so .123 can come back as .122.
	if claims.IssuedAt == nil || claims.IssuedAt.Add(time.Millisecond).Before(user.PasswordChangedAt.Truncate(time.Millisecond)) {
		return nil, ErrTokenRevoked
	}
	return user, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, claims *token.Claims, req model.UpdateProfileRequest) (*model.User, error) {
	if _, err := s.Me(ctx, claims); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, claims.UserID, req.Name, req.Image)
}

// ChangePassword logs out every device and returns a fresh session for the caller.
func (s *UserService) ChangePassword(ctx context.Context, claims *token.Claims, req model.ChangePasswordRequest) (*model.Session, error) {
	user, err := s.Me(ctx, claims)
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return nil, ErrWrongPassword
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// password_changed_at revokes old access tokens, RevokeAll old refresh tokens.
	if err := s.repo.UpdatePassword(ctx, user.ID, string(hashed), time.Now()); err != nil {
		return nil, err
	}
	if err := s.sessions.RevokeAll(ctx, user.ID); err != nil {
		return nil, err
	}
	// Created after the change, so its iat is not before password_changed_at.
	return s.startSession(ctx, user)
}
