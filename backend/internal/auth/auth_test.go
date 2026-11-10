package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubVerifier struct {
	identity Identity
	err      error
}

func (s stubVerifier) Verify(context.Context, string) (Identity, error) {
	return s.identity, s.err
}

type memoryDirectory struct {
	byGoogle map[string]User
	byID     map[string]User
	balance  map[string]int
	touched  []string
}

func (m *memoryDirectory) FindByGoogle(_ context.Context, subject string) (*User, error) {
	user, ok := m.byGoogle[subject]
	if !ok {
		return nil, nil
	}
	copy := user
	return &copy, nil
}

func (m *memoryDirectory) Get(_ context.Context, userID string) (*User, error) {
	user, ok := m.byID[userID]
	if !ok {
		return nil, nil
	}
	copy := user
	return &copy, nil
}

func (m *memoryDirectory) Create(_ context.Context, user User, googleSubject string) error {
	if m.byGoogle == nil {
		m.byGoogle = map[string]User{}
	}
	if m.byID == nil {
		m.byID = map[string]User{}
	}
	if m.balance == nil {
		m.balance = map[string]int{}
	}
	if _, exists := m.byGoogle[googleSubject]; exists {
		return errors.New("already exists")
	}
	m.byGoogle[googleSubject] = user
	m.byID[user.ID] = user
	m.balance[user.ID] = SignupErases
	return nil
}

func (m *memoryDirectory) TouchLogin(_ context.Context, userID string) error {
	m.touched = append(m.touched, userID)
	return nil
}

func (m *memoryDirectory) Balance(_ context.Context, userID string) (int, error) {
	return m.balance[userID], nil
}

func (m *memoryDirectory) Reserve(_ context.Context, userID string, pages int) error {
	if m.balance[userID] < pages {
		return ErrInsufficient
	}
	m.balance[userID] -= pages
	return nil
}

func (m *memoryDirectory) Refund(_ context.Context, userID string, pages int) error {
	if m.balance == nil {
		m.balance = map[string]int{}
	}
	m.balance[userID] += pages
	return nil
}

func TestSignInGoogleCreatesUser(t *testing.T) {
	dir := &memoryDirectory{}
	svc := NewService(stubVerifier{identity: Identity{Subject: "sub-1", Email: "a@b.com", Name: "Ada"}}, dir)
	user, err := svc.SignInGoogle(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "a@b.com" || user.Name != "Ada" || user.Remaining != SignupErases {
		t.Fatalf("unexpected user %#v", user)
	}
	parsed, err := uuid.Parse(user.ID)
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("expected uuid v7, got %q", user.ID)
	}
	again, err := svc.SignInGoogle(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != user.ID || again.Remaining != SignupErases {
		t.Fatal("expected same user on second login")
	}
	if len(dir.touched) != 1 {
		t.Fatalf("expected one touch, got %d", len(dir.touched))
	}
}

func TestReserveRejectsWhenEmpty(t *testing.T) {
	dir := &memoryDirectory{}
	svc := NewService(stubVerifier{identity: Identity{Subject: "sub-1", Email: "a@b.com", Name: "Ada"}}, dir)
	user, err := svc.SignInGoogle(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Reserve(context.Background(), user.ID, SignupErases); err != nil {
		t.Fatal(err)
	}
	if remaining, err := dir.Balance(context.Background(), user.ID); err != nil || remaining != 0 {
		t.Fatalf("remaining %d err %v", remaining, err)
	}
	if err := svc.Reserve(context.Background(), user.ID, 1); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("got %v", err)
	}
	if err := svc.Refund(context.Background(), user.ID, 1); err != nil {
		t.Fatal(err)
	}
	if remaining, err := dir.Balance(context.Background(), user.ID); err != nil || remaining != 1 {
		t.Fatalf("remaining %d err %v", remaining, err)
	}
}

func TestMeReturnsBalance(t *testing.T) {
	dir := &memoryDirectory{}
	svc := NewService(stubVerifier{identity: Identity{Subject: "sub-1", Email: "a@b.com", Name: "Ada"}}, dir)
	user, err := svc.SignInGoogle(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := svc.Me(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Remaining != SignupErases || profile.Email != "a@b.com" {
		t.Fatalf("unexpected profile %#v", profile)
	}
}

func TestSignInGoogleRejectsEmptyToken(t *testing.T) {
	svc := NewService(stubVerifier{}, &memoryDirectory{})
	if _, err := svc.SignInGoogle(context.Background(), "  "); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("got %v", err)
	}
}
