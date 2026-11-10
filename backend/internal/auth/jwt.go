package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenTTL = 3 * 24 * time.Hour

type Tokens struct {
	secret []byte
}

type Session struct {
	Profile
	Token string `json:"token"`
}

func NewTokens(secret string) *Tokens {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	return &Tokens{secret: []byte(secret)}
}

func (t *Tokens) Issue(userID string) (string, error) {
	if t == nil {
		return "", ErrUnavailable
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
	})
	return token.SignedString(t.secret)
}

func (t *Tokens) Parse(raw string) (string, error) {
	if t == nil {
		return "", ErrUnavailable
	}
	token, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return t.secret, nil
	})
	if err != nil || !token.Valid {
		return "", ErrUnauthorized
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return "", ErrUnauthorized
	}
	return claims.Subject, nil
}
