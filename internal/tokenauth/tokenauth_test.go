package tokenauth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amiranmanesh/payvand/internal/tokenauth"
)

func TestTokenIsFetchedOnceAndReused(t *testing.T) {
	var cache tokenauth.Cache
	calls := 0
	fetch := func(context.Context) (string, time.Duration, error) {
		calls++
		return "token-1", time.Hour, nil
	}

	for range 3 {
		token, err := cache.Token(context.Background(), fetch)
		if err != nil {
			t.Fatalf("Token() error = %v", err)
		}
		if token != "token-1" {
			t.Fatalf("token = %q", token)
		}
	}
	if calls != 1 {
		t.Fatalf("fetch called %d times, want 1", calls)
	}
}

func TestExpiredTokenIsFetchedAgain(t *testing.T) {
	var cache tokenauth.Cache
	calls := 0
	fetch := func(context.Context) (string, time.Duration, error) {
		calls++
		// A lifetime below the safety skew is already expired.
		return "token", time.Second, nil
	}

	if _, err := cache.Token(context.Background(), fetch); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if _, err := cache.Token(context.Background(), fetch); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("fetch called %d times, want 2", calls)
	}
}

func TestInvalidateForcesANewFetch(t *testing.T) {
	var cache tokenauth.Cache
	calls := 0
	fetch := func(context.Context) (string, time.Duration, error) {
		calls++
		return "token", time.Hour, nil
	}

	if _, err := cache.Token(context.Background(), fetch); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	cache.Invalidate()
	if _, err := cache.Token(context.Background(), fetch); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("fetch called %d times, want 2", calls)
	}
}

func TestFetchErrorIsReturnedAndNotCached(t *testing.T) {
	var cache tokenauth.Cache
	sentinel := errors.New("boom")

	if _, err := cache.Token(context.Background(), func(context.Context) (string, time.Duration, error) {
		return "", 0, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the fetch error", err)
	}

	token, err := cache.Token(context.Background(), func(context.Context) (string, time.Duration, error) {
		return "recovered", time.Hour, nil
	})
	if err != nil || token != "recovered" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}
