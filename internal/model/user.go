package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Password string    `json:"-"`
	Image    string    `json:"img_url"`
	IsActive bool      `json:"is_active"`
	// PasswordChangedAt is set by the app clock, the same clock that stamps token iat.
	PasswordChangedAt time.Time `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required,min=3,max=100"`
	Email    string `json:"email" binding:"required,email,max=100"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

func (r *RegisterRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Email = NormalizeEmail(r.Email)
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (r *LoginRequest) Normalize() {
	r.Email = NormalizeEmail(r.Email)
}

// UpdateProfileRequest is a partial update: a field left out of the JSON stays unchanged.
type UpdateProfileRequest struct {
	Name  *string `json:"name" binding:"omitnil,min=3,max=100"`
	Image *string `json:"img_url" binding:"omitnil,url,max=2048"`
}

func (r *UpdateProfileRequest) Normalize() {
	if r.Name != nil {
		name := strings.TrimSpace(*r.Name)
		r.Name = &name
	}
	if r.Image != nil {
		image := strings.TrimSpace(*r.Image)
		r.Image = &image
	}
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=72"`
}

// NormalizeEmail makes email comparison case-insensitive: "Budi@X.com" and "budi@x.com" are one account.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

type UserResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Image    string    `json:"img_url"`
	IsActive bool      `json:"is_active"`
}

type LoginResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}

func ToUserResponse(u *User) UserResponse {
	return UserResponse{
		ID:       u.ID,
		Name:     u.Name,
		Email:    u.Email,
		Image:    u.Image,
		IsActive: u.IsActive,
	}
}
