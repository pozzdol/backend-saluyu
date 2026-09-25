package service

import (
	"context"
	"errors"

	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/token"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken         = repository.ErrEmailTaken
	ErrInvalidCredentials = errors.New("password or email not match")
)

type UserService struct {
	repo  *repository.UserRepository
	token *token.Maker
}

func NewUserService(repo *repository.UserRepository, tm *token.Maker) *UserService {
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
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
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
