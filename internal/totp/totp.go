// Package totp implements RFC 6238 time-based one-time passwords (SHA-1, 30 s, 6 digits),
// the variant every authenticator app supports, and encrypts the shared secret at rest.
package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"time"
)

const period = 30

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns 160 random bits, the size RFC 4226 recommends.
func NewSecret() []byte {
	s := make([]byte, 20)
	rand.Read(s)
	return s
}

func Encode(secret []byte) string { return b32.EncodeToString(secret) }

// URI is what the QR code holds: otpauth://totp/Issuer:account?secret=...&issuer=Issuer
func URI(issuer, account string, secret []byte) string {
	q := url.Values{"secret": {Encode(secret)}, "issuer": {issuer}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}

// Code is the 6-digit code for one time step (Unix time / 30).
func Code(secret []byte, step int64) string {
	mac := hmac.New(sha1.New, secret)
	binary.Write(mac, binary.BigEndian, step)
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1_000_000)
}

// Match returns the time step the code belongs to, allowing one step of clock drift
// either way, or 0 when it matches none. The caller stores the step so it is used once.
func Match(secret []byte, code string, now time.Time) int64 {
	step := now.Unix() / period
	for _, s := range []int64{step, step - 1, step + 1} {
		if hmac.Equal([]byte(Code(secret, s)), []byte(code)) {
			return s
		}
	}
	return 0
}

// Box encrypts secrets with AES-256-GCM so a copy of the users table alone cannot make codes.
type Box struct{ aead cipher.AEAD }

// NewBox derives the key from the app secret; changing JWT_SECRET turns 2FA off for everyone.
func NewBox(appSecret string) *Box {
	key := sha256.Sum256([]byte("totp:" + appSecret))
	block, _ := aes.NewCipher(key[:])
	aead, _ := cipher.NewGCM(block)
	return &Box{aead: aead}
}

func (b *Box) Seal(secret []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	rand.Read(nonce)
	return b.aead.Seal(nonce, nonce, secret, nil)
}

func (b *Box) Open(sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("totp: sealed secret too short")
	}
	return b.aead.Open(nil, sealed[:n], sealed[n:], nil)
}
