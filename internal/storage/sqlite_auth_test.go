package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/auth"
)

func TestSQLiteAccountAndSession(t *testing.T) {
	store, err := NewSQLite(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	expires := now.Add(30 * time.Minute)
	value := auth.UserRecord{User: auth.User{ID: "user_1", Name: "James", Email: "james@example.com", CreatedAt: now, UpdatedAt: now}, PasswordHash: "hash", VerificationTokenHash: "verify", VerificationTokenExpiry: &expires}
	if err := store.CreateUser(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(context.Background(), value); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("duplicate error = %v", err)
	}
	loaded, err := store.GetUserByVerificationToken(context.Background(), "verify")
	if err != nil || loaded.Email != value.Email {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
	if err := store.MarkUserVerified(context.Background(), value.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(context.Background(), auth.SessionRecord{TokenHash: "session", UserID: value.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	user, err := store.GetUserBySession(context.Background(), "session", now)
	if err != nil || !user.EmailVerified {
		t.Fatalf("session user = %#v, %v", user, err)
	}
}
