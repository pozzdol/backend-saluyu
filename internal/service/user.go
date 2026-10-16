package service

import (
	"context"
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

type UserService struct {
	repo  UserRepository
	token *token.Maker
}

func NewUserService(repo UserRepository, tm *token.Maker) *UserService {
	return &UserService{repo: repo, token: tm}
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

func (s *UserService) Login(ctx context.Context, req model.LoginRequest) (*model.LoginResponse, error) {
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

	signed, err := s.token.Create(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &model.LoginResponse{
		Token: signed,
		User:  model.ToUserResponse(user),
	}, nil
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
	if claims.IssuedAt == nil || claims.IssuedAt.Before(user.PasswordChangedAt.Truncate(time.Millisecond)) {
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

// ChangePassword revokes every token issued before now and returns a fresh one for the caller.
func (s *UserService) ChangePassword(ctx context.Context, claims *token.Claims, req model.ChangePasswordRequest) (string, error) {
	user, err := s.Me(ctx, claims)
	if err != nil {
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return "", ErrWrongPassword
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	if err := s.repo.UpdatePassword(ctx, user.ID, string(hashed), time.Now()); err != nil {
		return "", err
	}
	// Created after the change, so its iat is not before password_changed_at.
	return s.token.Create(user.ID, user.Email)
}
