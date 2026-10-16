package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/mail"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/token"
	"github.com/pozzdol/backend-saluyu/internal/totp"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidLink        = repository.ErrInvalidLink
	ErrSessionNotFound    = repository.ErrSessionNotFound
	ErrEmailUnverified    = repository.ErrEmailUnverified
	ErrMFAInvalid         = errors.New("sign-in step expired, please log in again")
	ErrInvalidCode        = errors.New("invalid code")
	ErrTOTPNotSetup       = errors.New("two-factor setup was not started")
	ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already on")
)

const (
	purposeVerify = "verify"
	purposeReset  = "reset"
	backupCodes   = 10
)

var linkTTL = map[string]time.Duration{purposeVerify: 48 * time.Hour, purposeReset: 30 * time.Minute}

// ForgotPassword emails a reset link when the email belongs to an active user. It answers the
// same either way and sends in the background, so the response never tells who has an account.
func (s *UserService) ForgotPassword(ctx context.Context, email, lang string) error {
	user, err := s.repo.FindByEmail(ctx, email)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !user.IsActive {
		return nil
	}
	return s.sendLink(ctx, user, purposeReset, lang)
}

// ResetPassword sets a new password from a reset link and ends every session. Opening the
// link proves the user reads that inbox, so the email counts as verified too.
func (s *UserService) ResetPassword(ctx context.Context, req model.ResetPasswordRequest) error {
	userID, err := s.security.UseEmailToken(ctx, purposeReset, hashToken(req.Token))
	if err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(ctx, userID, string(hashed), time.Now()); err != nil {
		return err
	}
	if err := s.sessions.RevokeAll(ctx, userID, uuid.Nil); err != nil {
		return err
	}
	return s.repo.SetEmailVerified(ctx, userID)
}

func (s *UserService) VerifyEmail(ctx context.Context, tok string) error {
	userID, err := s.security.UseEmailToken(ctx, purposeVerify, hashToken(tok))
	if err != nil {
		return err
	}
	return s.repo.SetEmailVerified(ctx, userID)
}

// ResendVerification sends a new link; an already verified user gets nothing.
func (s *UserService) ResendVerification(ctx context.Context, claims *token.Claims, lang string) error {
	user, err := s.Me(ctx, claims)
	if err != nil || user.EmailVerifiedAt != nil {
		return err
	}
	return s.sendLink(ctx, user, purposeVerify, lang)
}

func (s *UserService) sendLink(ctx context.Context, user *model.User, purpose, lang string) error {
	raw := rand.Text()
	if err := s.security.CreateEmailToken(ctx, user.ID, purpose, hashToken(raw), time.Now().Add(linkTTL[purpose])); err != nil {
		return err
	}
	if user.Locale != nil {
		lang = *user.Locale
	}
	msg := linkEmail(purpose, lang, user.Name, s.appURL, raw)
	msg.To = user.Email
	go func() { // detached from the request: it may end before the email API answers
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, msg); err != nil {
			log.Printf("send %s email: %v", purpose, err)
		}
	}()
	return nil
}

func linkEmail(purpose, lang, name, appURL, tok string) mail.Message {
	if lang != "id" {
		lang = "en"
	}
	path := map[string]string{purposeVerify: "/verify-email", purposeReset: "/reset-password"}[purpose]
	// subject, title, body, button, expiry, ignore
	texts := map[string][6]string{
		"reset/id": {"Atur ulang password Saluyu", "Atur ulang password",
			"Kami menerima permintaan untuk mengatur ulang password akun Saluyu-mu.",
			"Atur password baru", "Link ini berlaku 30 menit.",
			"Kalau kamu tidak memintanya, abaikan email ini. Password-mu tidak berubah."},
		"reset/en": {"Reset your Saluyu password", "Reset your password",
			"We received a request to reset the password of your Saluyu account.",
			"Set a new password", "This link works for 30 minutes.",
			"If you did not ask for this, ignore this email. Your password stays the same."},
		"verify/id": {"Verifikasi email Saluyu", "Verifikasi emailmu",
			"Satu langkah lagi: pastikan email ini milikmu, supaya link reset password bisa sampai kalau kamu lupa password.",
			"Verifikasi email", "Link ini berlaku 48 jam.",
			"Kalau kamu tidak mendaftar di Saluyu, abaikan email ini."},
		"verify/en": {"Verify your Saluyu email", "Verify your email",
			"One more step: confirm this email is yours, so a reset link can reach you if you forget your password.",
			"Verify email", "This link works for 48 hours.",
			"If you did not sign up for Saluyu, ignore this email."},
	}
	t := texts[purpose+"/"+lang]
	greeting, fallback := "Hi %s,", "Button not working? Paste this link into your browser:"
	if lang == "id" {
		greeting, fallback = "Halo %s,", "Tombol tidak bisa dibuka? Salin link ini ke browser:"
	}
	return mail.Link{
		Lang: lang, AppURL: appURL, Subject: t[0], Title: t[1], Greeting: fmt.Sprintf(greeting, name), Body: t[2],
		Button: t[3], URL: appURL + path + "?token=" + tok, Expiry: t[4], Fallback: fallback, Ignore: t[5],
	}.Message()
}

// ListSessions returns the live sessions; refreshToken (the caller's cookie, may be empty) marks the current one.
func (s *UserService) ListSessions(ctx context.Context, claims *token.Claims, refreshToken string) ([]model.SessionInfo, error) {
	if _, err := s.Me(ctx, claims); err != nil {
		return nil, err
	}
	list, err := s.sessions.List(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if current, err := s.sessions.SessionOf(ctx, hashToken(refreshToken)); err == nil {
		for i := range list {
			list[i].Current = list[i].ID == current
		}
	}
	return list, nil
}

func (s *UserService) EndSession(ctx context.Context, claims *token.Claims, sessionID uuid.UUID) error {
	if _, err := s.Me(ctx, claims); err != nil {
		return err
	}
	return s.sessions.End(ctx, claims.UserID, sessionID)
}

// RevokeOtherSessions ends every session but the caller's and stops all older access tokens.
// It returns a fresh access token for the caller, issued after the cutoff.
func (s *UserService) RevokeOtherSessions(ctx context.Context, claims *token.Claims, refreshToken string) (string, error) {
	user, err := s.Me(ctx, claims)
	if err != nil {
		return "", err
	}
	current, err := s.sessions.SessionOf(ctx, hashToken(refreshToken))
	if err != nil {
		return "", err
	}
	if err := s.sessions.RevokeAll(ctx, user.ID, current); err != nil {
		return "", err
	}
	if err := s.repo.RevokeTokensBefore(ctx, user.ID, time.Now()); err != nil {
		return "", err
	}
	return s.token.Create(user.ID, user.Email)
}

// SetupTOTP starts 2FA setup with a new secret; it is not required at login until EnableTOTP.
func (s *UserService) SetupTOTP(ctx context.Context, claims *token.Claims) (*model.TOTPSetup, error) {
	user, err := s.Me(ctx, claims)
	if err != nil {
		return nil, err
	}
	if user.TOTPEnabledAt != nil {
		return nil, ErrTOTPAlreadyEnabled
	}
	secret := totp.NewSecret()
	if err := s.repo.SetTOTP(ctx, user.ID, s.totp.Seal(secret), nil, 0); err != nil {
		return nil, err
	}
	return &model.TOTPSetup{Secret: totp.Encode(secret), URI: totp.URI("Saluyu", user.Email, secret)}, nil
}

// EnableTOTP turns 2FA on once the app shows a matching code, and returns the backup codes.
// They are shown only this once.
func (s *UserService) EnableTOTP(ctx context.Context, claims *token.Claims, code string) ([]string, error) {
	user, err := s.Me(ctx, claims)
	if err != nil {
		return nil, err
	}
	if user.TOTPEnabledAt != nil {
		return nil, ErrTOTPAlreadyEnabled
	}
	if user.TOTPSecret == nil {
		return nil, ErrTOTPNotSetup
	}
	secret, err := s.totp.Open(user.TOTPSecret)
	if err != nil {
		return nil, err
	}
	step := totp.Match(secret, normalizeCode(code), time.Now())
	if step == 0 {
		return nil, ErrInvalidCode
	}

	codes := make([]string, backupCodes)
	hashes := make([][]byte, backupCodes)
	for i := range codes {
		c := strings.ToLower(rand.Text()[:10]) // 50 random bits
		codes[i] = c[:5] + "-" + c[5:]
		hashes[i] = hashToken(c)
	}
	if err := s.security.ReplaceBackupCodes(ctx, user.ID, hashes); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.repo.SetTOTP(ctx, user.ID, user.TOTPSecret, &now, step); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *UserService) DisableTOTP(ctx context.Context, claims *token.Claims, req model.DisableTOTPRequest) error {
	user, err := s.Me(ctx, claims)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		return ErrWrongPassword
	}
	if user.TOTPEnabledAt == nil {
		return ErrTOTPNotSetup
	}
	if err := s.secondFactor(ctx, user, req.Code); err != nil {
		return err
	}
	if err := s.repo.SetTOTP(ctx, user.ID, nil, nil, 0); err != nil {
		return err
	}
	return s.security.ReplaceBackupCodes(ctx, user.ID, nil)
}

// LoginTOTP finishes a 2FA login: the mfa ticket from Login plus a code from the app or a backup code.
func (s *UserService) LoginTOTP(ctx context.Context, req model.TOTPLoginRequest, userAgent string) (*model.Session, error) {
	claims, err := s.token.Verify(req.MFAToken)
	if err != nil || claims.Purpose != token.PurposeMFA {
		return nil, ErrMFAInvalid
	}
	user, err := s.repo.FindByID(ctx, claims.UserID)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrMFAInvalid
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrUserInactive
	}
	if user.TOTPEnabledAt == nil {
		return nil, ErrMFAInvalid
	}
	if err := s.secondFactor(ctx, user, req.Code); err != nil {
		return nil, err
	}
	return s.startSession(ctx, user, uuid.Nil, userAgent)
}

// secondFactor accepts a 6-digit code (each time step once) or an unused backup code.
func (s *UserService) secondFactor(ctx context.Context, user *model.User, code string) error {
	code = normalizeCode(code)
	if len(code) == 6 && strings.Trim(code, "0123456789") == "" {
		secret, err := s.totp.Open(user.TOTPSecret)
		if err != nil {
			return err
		}
		step := totp.Match(secret, code, time.Now())
		if step == 0 {
			return ErrInvalidCode
		}
		fresh, err := s.repo.UseTOTPStep(ctx, user.ID, step)
		if err != nil {
			return err
		}
		if !fresh {
			return ErrInvalidCode // replayed
		}
		return nil
	}
	ok, err := s.security.UseBackupCode(ctx, user.ID, hashToken(code))
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCode
	}
	return nil
}

// normalizeCode drops spaces and dashes and lowercases, so "abcde-fghij" and "123 456" both work.
func normalizeCode(c string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(c)))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
