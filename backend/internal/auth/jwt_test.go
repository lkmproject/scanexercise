package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueTokenExpiresInThreeDays(t *testing.T) {
	tokens := NewTokens("test-secret")
	raw, err := tokens.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := tokens.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "user-1" {
		t.Fatalf("got %q", userID)
	}
	parsed, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(*jwt.Token) (any, error) {
		return tokens.secret, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	claims := parsed.Claims.(*jwt.RegisteredClaims)
	ttl := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if ttl < TokenTTL-time.Second || ttl > TokenTTL+time.Second {
		t.Fatalf("ttl %s", ttl)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	tokens := NewTokens("test-secret")
	raw, err := tokens.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTokens("other-secret").Parse(raw); err == nil {
		t.Fatal("expected error")
	}
}
