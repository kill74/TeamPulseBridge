package securityaudit

import (
	"context"
	"errors"
	"testing"
)

func TestAsyncStoreUnavailableInner(t *testing.T) {
	var pgStore *PostgresStore
	cases := map[string]Store{
		"nil interface": nil,
		"typed nil":     pgStore,
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			for _, closed := range []bool{false, true} {
				a := NewAsyncStore(inner, 1)
				if closed {
					a.Close()
				}
				ctx := context.Background()

				in := SaveInput{Category: "request_rejected", Outcome: "rejected", Source: "github", Reason: "test", Path: "/webhook", HTTPStatus: 401}
				if _, err := a.Save(ctx, in); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("Save: expected ErrStoreUnavailable, got %v", err)
				}
				if _, err := a.ListRecent(ctx, 1); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("ListRecent: expected ErrStoreUnavailable, got %v", err)
				}
				a.Close()
			}
		})
	}
}
