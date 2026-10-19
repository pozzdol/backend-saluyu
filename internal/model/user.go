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
	// StartDate is the day the books open; nil until the opening balance is set.
	StartDate *time.Time `json:"-"`
	// PeriodStartDay is the payday (1..31) on which each budgeting period starts.
	PeriodStartDay int `json:"-"`
	// PeriodMode picks how periods are cut (see period.Config); PeriodWeekday is for weekly.
	PeriodMode    string `json:"-"`
	PeriodWeekday int    `json:"-"`
	// Locale is "id" or "en"; nil means follow the browser.
	Locale *string `json:"-"`
	// EmailVerifiedAt is nil until the user opens the link in the verification email.
	EmailVerifiedAt *time.Time `json:"-"`
	// SessionsRevokedAt rejects access tokens issued before it ("log out everywhere").
	SessionsRevokedAt *time.Time `json:"-"`
	// TOTPSecret is encrypted; TOTPEnabledAt stays nil until setup is confirmed with a code.
	TOTPSecret    []byte     `json:"-"`
	TOTPEnabledAt *time.Time `json:"-"`
	// LockedUntil closes the books up to and including this day.
	LockedUntil *time.Time `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
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
	// Changing it re-slices past periods too: periods are computed, never stored.
	PeriodStartDay *int    `json:"period_start_day" binding:"omitnil,min=1,max=31"`
	PeriodMode     *string `json:"period_mode" binding:"omitnil,oneof=payday last_workday calendar_month weekly"`
	PeriodWeekday  *int    `json:"period_weekday" binding:"omitnil,min=0,max=6"`
	// Locale "auto" clears the choice so the app follows the browser again.
	Locale *string `json:"locale" binding:"omitnil,oneof=id en auto"`
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
	// StartDate as YYYY-MM-DD, or null: the frontend sends a new user to the opening-balance page.
	StartDate      *string `json:"start_date"`
	PeriodStartDay int     `json:"period_start_day"`
	PeriodMode     string  `json:"period_mode"`
	PeriodWeekday  int     `json:"period_weekday"`
	Locale         *string `json:"locale"` // null = follow the browser
	EmailVerified  bool    `json:"email_verified"`
	TOTPEnabled    bool    `json:"totp_enabled"`
	LockedUntil    *string `json:"locked_until"`
}

// Session is what login and refresh hand out. The refresh token never goes in the JSON body:
// the handler puts it in an HttpOnly cookie that page scripts cannot read.
type Session struct {
	AccessToken string       `json:"access_token,omitempty"`
	User        UserResponse `json:"user"`
	// MFARequired means the password was right and a TOTP code is needed next:
	// the client sends MFAToken with the code to /login/totp. No other field is set then.
	MFARequired    bool      `json:"mfa_required,omitempty"`
	MFAToken       string    `json:"mfa_token,omitempty"`
	RefreshToken   string    `json:"-"`
	RefreshExpires time.Time `json:"-"`
}

func ToUserResponse(u *User) UserResponse {
	return UserResponse{
		ID:             u.ID,
		Name:           u.Name,
		Email:          u.Email,
		Image:          u.Image,
		IsActive:       u.IsActive,
		StartDate:      DateString(u.StartDate),
		PeriodStartDay: u.PeriodStartDay,
		PeriodMode:     u.PeriodMode,
		PeriodWeekday:  u.PeriodWeekday,
		Locale:         u.Locale,
		EmailVerified:  u.EmailVerifiedAt != nil,
		TOTPEnabled:    u.TOTPEnabledAt != nil,
		LockedUntil:    DateString(u.LockedUntil),
	}
}

// DateString formats an optional calendar date as YYYY-MM-DD.
func DateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

// TOTPLoginRequest is the second step of a 2FA login.
type TOTPLoginRequest struct {
	MFAToken string `json:"mfa_token" binding:"required"`
	Code     string `json:"code" binding:"required,max=20"` // 6 digits, or a backup code
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email,max=100"`
}

func (r *ForgotPasswordRequest) Normalize() { r.Email = NormalizeEmail(r.Email) }

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required,max=100"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=72"`
}

type TokenRequest struct {
	Token string `json:"token" binding:"required,max=100"`
}

type CodeRequest struct {
	Code string `json:"code" binding:"required,max=20"`
}

type DisableTOTPRequest struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required,max=20"`
}

// SessionInfo is one logged-in device: a chain of rotated refresh tokens.
type SessionInfo struct {
	ID         uuid.UUID `json:"id"`
	UserAgent  string    `json:"user_agent"`
	StartedAt  time.Time `json:"started_at"`
	LastActive time.Time `json:"last_active"`
	Current    bool      `json:"current"`
}

type TOTPSetup struct {
	Secret string `json:"secret"` // base32, for typing into the app by hand
	URI    string `json:"uri"`    // otpauth:// URI, shown as a QR code
}

// LockRequest closes the books up to a day; null opens them again.
type LockRequest struct {
	LockedUntil *string `json:"locked_until" binding:"omitnil,datetime=2006-01-02"`
}
