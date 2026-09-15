package service

import (
	"context"
	"errors"

	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var ErrEmailTaken = errors.New("email already taken")

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
	existing, err := s.repo.FindByEmail(ctx, req.Email)

	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
		return nil, err
	}

	if existing != nil {
		return nil, ErrEmailTaken
	}

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

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}
