package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/id"
)

var (
	ErrNotFound          = errors.New("account not found")
	ErrEmailTaken        = errors.New("email address is already registered")
	ErrInvalidCredential = errors.New("invalid email address or password")
	ErrEmailNotVerified  = errors.New("email address has not been verified")
	ErrInvalidToken      = errors.New("verification link is invalid or expired")
)

const sessionTTL = 30 * 24 * time.Hour

type User struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UserRecord struct {
	User
	PasswordHash            string
	VerificationTokenHash   string
	VerificationTokenExpiry *time.Time
}

type SessionRecord struct {
	TokenHash string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store interface {
	CreateUser(context.Context, UserRecord) error
	DeleteUser(context.Context, string) error
	GetUserByID(context.Context, string) (UserRecord, error)
	GetUserByEmail(context.Context, string) (UserRecord, error)
	GetUserByVerificationToken(context.Context, string) (UserRecord, error)
	MarkUserVerified(context.Context, string, time.Time) error
	CreateSession(context.Context, SessionRecord) error
	GetUserBySession(context.Context, string, time.Time) (UserRecord, error)
	DeleteSession(context.Context, string) error
	DeleteExpiredSessions(context.Context, time.Time) error
}

type VerificationMailer interface {
	Configured() bool
	SendVerification(context.Context, string, string, string) error
}

type Service struct {
	store     Store
	mailer    VerificationMailer
	publicURL string
	now       func() time.Time
}

func NewService(store Store, mailer VerificationMailer, publicURL string) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("auth store is required")
	}
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	parsed, err := url.Parse(publicURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("public URL must be an absolute HTTP or HTTPS URL")
	}
	return &Service{store: store, mailer: mailer, publicURL: publicURL, now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, name, email, password string) (User, error) {
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 80 {
		return User{}, fmt.Errorf("name must contain between 1 and 80 characters")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	if s.mailer == nil || !s.mailer.Configured() {
		return User{}, fmt.Errorf("email delivery must be configured before accounts can be registered")
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return User{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(30 * time.Minute)
	record := UserRecord{
		User:         User{ID: id.New("user"), Name: name, Email: email, CreatedAt: now, UpdatedAt: now},
		PasswordHash: passwordHash, VerificationTokenHash: tokenHash, VerificationTokenExpiry: &expiresAt,
	}
	if err := s.store.CreateUser(ctx, record); err != nil {
		return User{}, err
	}
	verificationURL := s.publicURL + "/#/verify-email?token=" + url.QueryEscape(token)
	if err := s.mailer.SendVerification(ctx, email, name, verificationURL); err != nil {
		_ = s.store.DeleteUser(ctx, record.ID)
		return User{}, fmt.Errorf("send verification email: %w", err)
	}
	return record.User, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) (User, error) {
	hash := hashToken(strings.TrimSpace(token))
	if hash == "" {
		return User{}, ErrInvalidToken
	}
	record, err := s.store.GetUserByVerificationToken(ctx, hash)
	if err != nil || record.VerificationTokenExpiry == nil || !record.VerificationTokenExpiry.After(s.now().UTC()) {
		return User{}, ErrInvalidToken
	}
	now := s.now().UTC()
	if err := s.store.MarkUserVerified(ctx, record.ID, now); err != nil {
		return User{}, err
	}
	record.EmailVerified = true
	record.UpdatedAt = now
	return record.User, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (User, string, time.Time, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, "", time.Time{}, ErrInvalidCredential
	}
	record, err := s.store.GetUserByEmail(ctx, email)
	if err != nil || !VerifyPassword(record.PasswordHash, password) {
		return User{}, "", time.Time{}, ErrInvalidCredential
	}
	if !record.EmailVerified {
		return User{}, "", time.Time{}, ErrEmailNotVerified
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(sessionTTL)
	_ = s.store.DeleteExpiredSessions(ctx, now)
	if err := s.store.CreateSession(ctx, SessionRecord{TokenHash: tokenHash, UserID: record.ID, CreatedAt: now, ExpiresAt: expiresAt}); err != nil {
		return User{}, "", time.Time{}, err
	}
	return record.User, token, expiresAt, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	hash := hashToken(strings.TrimSpace(token))
	if hash == "" {
		return User{}, ErrNotFound
	}
	record, err := s.store.GetUserBySession(ctx, hash, s.now().UTC())
	if err != nil {
		return User{}, err
	}
	return record.User, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	hash := hashToken(strings.TrimSpace(token))
	if hash == "" {
		return nil
	}
	if err := s.store.DeleteSession(ctx, hash); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 {
		return "", fmt.Errorf("a valid email address is required")
	}
	return value, nil
}

func newToken() (string, string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", "", fmt.Errorf("generate secure token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(value)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	if token == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
