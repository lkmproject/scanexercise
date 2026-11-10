package auth

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"
)

type GoogleVerifier struct {
	audience string
}

func NewGoogleVerifier(audience string) *GoogleVerifier {
	return &GoogleVerifier{audience: strings.TrimSpace(audience)}
}

func (v *GoogleVerifier) Verify(ctx context.Context, idToken string) (Identity, error) {
	if v.audience == "" {
		return Identity{}, fmt.Errorf("missing Google Web Client ID")
	}
	payload, err := idtoken.Validate(ctx, idToken, v.audience)
	if err != nil {
		return Identity{}, err
	}
	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	return Identity{
		Subject: payload.Subject,
		Email:   email,
		Name:    name,
	}, nil
}
