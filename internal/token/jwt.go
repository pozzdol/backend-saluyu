package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

func init() {
	// Millisecond iat, not whole seconds: a password changed in the same second as a login
	// must still revoke that login's token. Fractional NumericDate is allowed by RFC 7519.
	jwt.TimePrecision = time.Millisecond
}

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	// Purpose is empty for access tokens. "mfa" marks the short-lived ticket between the
	// password and the 2FA code; Auth rejects it, so it never opens the API.
	Purpose string `json:"pur,omitempty"`
	jwt.RegisteredClaims
}

type Maker struct {
	secret   []byte
	duration time.Duration
}

func NewMaker(secret string, duration time.Duration) *Maker {
	return &Maker{secret: []byte(secret), duration: duration}
}

func (m *Maker) Create(userID uuid.UUID, email string) (string, error) {
	return m.sign(userID, email, "", m.duration)
}

// PurposeMFA tokens prove the password was right; they last 5 minutes.
const PurposeMFA = "mfa"

func (m *Maker) CreateMFA(userID uuid.UUID, email string) (string, error) {
	return m.sign(userID, email, PurposeMFA, 5*time.Minute)
}

func (m *Maker) sign(userID uuid.UUID, email, purpose string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:  userID,
		Email:   email,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *Maker) Verify(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
