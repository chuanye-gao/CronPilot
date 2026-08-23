package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(encoded, "wrong password") {
		t.Fatal("wrong password verified")
	}
}

func TestRegisterVerifyLoginSession(t *testing.T) {
	store := newTestStore()
	mailer := &testMailer{configured: true}
	service, err := NewService(store, mailer, "http://127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) }

	user, err := service.Register(context.Background(), "James", "James@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "james@example.com" || mailer.to != user.Email {
		t.Fatalf("user = %#v, mail recipient = %q", user, mailer.to)
	}
	if _, _, _, err := service.Login(context.Background(), user.Email, "password123"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("login before verification error = %v", err)
	}
	token := strings.TrimPrefix(mailer.link, "http://127.0.0.1:18080/#/verify-email?token=")
	verified, err := service.VerifyEmail(context.Background(), token)
	if err != nil || !verified.EmailVerified {
		t.Fatalf("verify = %#v, %v", verified, err)
	}
	loggedIn, sessionToken, expiresAt, err := service.Login(context.Background(), user.Email, "password123")
	if err != nil || sessionToken == "" || !expiresAt.After(service.now()) {
		t.Fatalf("login = %#v, token=%q, expiry=%v, err=%v", loggedIn, sessionToken, expiresAt, err)
	}
	authenticated, err := service.Authenticate(context.Background(), sessionToken)
	if err != nil || authenticated.ID != user.ID {
		t.Fatalf("authenticate = %#v, %v", authenticated, err)
	}
	if err := service.Logout(context.Background(), sessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), sessionToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("authenticate after logout error = %v", err)
	}
}

type testMailer struct {
	configured bool
	to         string
	link       string
}

func (m *testMailer) Configured() bool { return m.configured }
func (m *testMailer) SendVerification(_ context.Context, to, _ string, link string) error {
	m.to, m.link = to, link
	return nil
}

type testStore struct {
	users    map[string]UserRecord
	emails   map[string]string
	tokens   map[string]string
	sessions map[string]SessionRecord
}

func newTestStore() *testStore {
	return &testStore{users: map[string]UserRecord{}, emails: map[string]string{}, tokens: map[string]string{}, sessions: map[string]SessionRecord{}}
}

func (s *testStore) CreateUser(_ context.Context, value UserRecord) error {
	if _, exists := s.emails[value.Email]; exists {
		return ErrEmailTaken
	}
	s.users[value.ID] = value
	s.emails[value.Email] = value.ID
	s.tokens[value.VerificationTokenHash] = value.ID
	return nil
}
func (s *testStore) DeleteUser(_ context.Context, id string) error {
	value, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	delete(s.emails, value.Email)
	delete(s.tokens, value.VerificationTokenHash)
	return nil
}
func (s *testStore) GetUserByID(_ context.Context, id string) (UserRecord, error) {
	value, ok := s.users[id]
	if !ok {
		return UserRecord{}, ErrNotFound
	}
	return value, nil
}
func (s *testStore) GetUserByEmail(_ context.Context, email string) (UserRecord, error) {
	id, ok := s.emails[email]
	if !ok {
		return UserRecord{}, ErrNotFound
	}
	return s.users[id], nil
}
func (s *testStore) GetUserByVerificationToken(_ context.Context, token string) (UserRecord, error) {
	id, ok := s.tokens[token]
	if !ok {
		return UserRecord{}, ErrNotFound
	}
	return s.users[id], nil
}
func (s *testStore) MarkUserVerified(_ context.Context, id string, now time.Time) error {
	value, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.tokens, value.VerificationTokenHash)
	value.EmailVerified = true
	value.VerificationTokenHash = ""
	value.VerificationTokenExpiry = nil
	value.UpdatedAt = now
	s.users[id] = value
	return nil
}
func (s *testStore) CreateSession(_ context.Context, value SessionRecord) error {
	s.sessions[value.TokenHash] = value
	return nil
}
func (s *testStore) GetUserBySession(_ context.Context, token string, now time.Time) (UserRecord, error) {
	session, ok := s.sessions[token]
	if !ok || !session.ExpiresAt.After(now) {
		return UserRecord{}, ErrNotFound
	}
	return s.GetUserByID(context.Background(), session.UserID)
}
func (s *testStore) DeleteSession(_ context.Context, token string) error {
	if _, ok := s.sessions[token]; !ok {
		return ErrNotFound
	}
	delete(s.sessions, token)
	return nil
}
func (s *testStore) DeleteExpiredSessions(_ context.Context, now time.Time) error {
	for token, value := range s.sessions {
		if !value.ExpiresAt.After(now) {
			delete(s.sessions, token)
		}
	}
	return nil
}
