package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const SignupErases = 2

var (
	ErrInvalidToken = errors.New("invalid google sign-in")
	ErrUnavailable  = errors.New("auth service unavailable")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInsufficient = errors.New("not enough erases")
)

type User struct {
	ID    string `json:"user_id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Profile struct {
	User
	Remaining int `json:"remaining"`
}

type Identity struct {
	Subject string
	Email   string
	Name    string
}

type Verifier interface {
	Verify(ctx context.Context, idToken string) (Identity, error)
}

type Directory interface {
	FindByGoogle(ctx context.Context, subject string) (*User, error)
	Get(ctx context.Context, userID string) (*User, error)
	Create(ctx context.Context, user User, googleSubject string) error
	TouchLogin(ctx context.Context, userID string) error
	Balance(ctx context.Context, userID string) (int, error)
	Reserve(ctx context.Context, userID string, pages int) error
	Refund(ctx context.Context, userID string, pages int) error
}

type Service struct {
	verifier  Verifier
	directory Directory
}

func NewService(verifier Verifier, directory Directory) *Service {
	if verifier == nil || directory == nil {
		return nil
	}
	return &Service{verifier: verifier, directory: directory}
}

func (s *Service) Me(ctx context.Context, userID string) (*Profile, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	user, err := s.directory.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUnauthorized
	}
	remaining, err := s.directory.Balance(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &Profile{User: *user, Remaining: remaining}, nil
}

func (s *Service) Reserve(ctx context.Context, userID string, pages int) error {
	if s == nil {
		return ErrUnavailable
	}
	if pages <= 0 {
		return nil
	}
	return s.directory.Reserve(ctx, userID, pages)
}

func (s *Service) Balance(ctx context.Context, userID string) (int, error) {
	if s == nil {
		return 0, ErrUnavailable
	}
	return s.directory.Balance(ctx, userID)
}

func (s *Service) Refund(ctx context.Context, userID string, pages int) error {
	if s == nil {
		return ErrUnavailable
	}
	if pages <= 0 {
		return nil
	}
	return s.directory.Refund(ctx, userID, pages)
}

func (s *Service) SignInGoogle(ctx context.Context, idToken string) (*Profile, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return nil, ErrInvalidToken
	}
	identity, err := s.verifier.Verify(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if identity.Subject == "" {
		return nil, ErrInvalidToken
	}
	user, err := s.directory.FindByGoogle(ctx, identity.Subject)
	if err != nil {
		return nil, err
	}
	if user != nil {
		_ = s.directory.TouchLogin(ctx, user.ID)
		return s.profile(ctx, user)
	}
	user = &User{
		ID:    newID(),
		Email: identity.Email,
		Name:  displayName(identity),
	}
	if err := s.directory.Create(ctx, *user, identity.Subject); err != nil {
		existing, findErr := s.directory.FindByGoogle(ctx, identity.Subject)
		if findErr == nil && existing != nil {
			return s.profile(ctx, existing)
		}
		return nil, err
	}
	return s.profile(ctx, user)
}

func (s *Service) profile(ctx context.Context, user *User) (*Profile, error) {
	remaining, err := s.directory.Balance(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &Profile{User: *user, Remaining: remaining}, nil
}

func displayName(identity Identity) string {
	if strings.TrimSpace(identity.Name) != "" {
		return identity.Name
	}
	if identity.Email != "" {
		return identity.Email
	}
	return "User"
}

func newID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}
