package totp

import (
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA-1 column (the RFC prints 8 digits; we keep the last 6).
func TestRFC6238(t *testing.T) {
	secret := []byte("12345678901234567890")
	for unix, want := range map[int64]string{
		59: "287082", 1111111109: "081804", 1111111111: "050471",
		1234567890: "005924", 2000000000: "279037", 20000000000: "353130",
	} {
		if got := Code(secret, unix/period); got != want {
			t.Errorf("t=%d: got %s want %s", unix, got, want)
		}
	}
}

func TestMatchDrift(t *testing.T) {
	secret := NewSecret()
	now := time.Unix(1_700_000_000, 0)
	step := now.Unix() / period
	for _, s := range []int64{step - 1, step, step + 1} {
		if got := Match(secret, Code(secret, s), now); got != s {
			t.Errorf("step %d: matched %d", s, got)
		}
	}
	if Match(secret, Code(secret, step+2), now) != 0 {
		t.Error("code two steps ahead must not match")
	}
}

func TestBox(t *testing.T) {
	b, secret := NewBox("app"), NewSecret()
	sealed := b.Seal(secret)
	got, err := b.Open(sealed)
	if err != nil || string(got) != string(secret) {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, err := NewBox("other").Open(sealed); err == nil {
		t.Error("another key must not open the secret")
	}
}
