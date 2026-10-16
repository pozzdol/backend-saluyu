package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestBlockedByReference(t *testing.T) {
	for code, want := range map[string]bool{"23001": true, "23503": true, "23505": false} {
		err := fmt.Errorf("delete: %w", &pgconn.PgError{Code: code})
		if got := blockedByReference(err); got != want {
			t.Errorf("code %s: got %v, want %v", code, got, want)
		}
	}
	if blockedByReference(nil) || blockedByReference(errors.New("other")) {
		t.Error("non-postgres errors must not count as a blocked delete")
	}
}
