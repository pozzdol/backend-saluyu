package token

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateVerify(t *testing.T) {
	m := NewMaker("secret", time.Minute)
	id := uuid.New()

	signed, err := m.Create(id, "a@b.c")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := m.Verify(signed)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != id || claims.Email != "a@b.c" {
		t.Fatalf("claims mismatch: %+v", claims)
	}

	if _, err := NewMaker("other", time.Minute).Verify(signed); err == nil {
		t.Fatal("expected invalid token with wrong secret")
	}
}
