package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/auth"
)

const userSelect = `SELECT users.id, users.name, users.email, users.password_hash, users.email_verified,
    users.verification_token_hash, users.verification_expires_at, users.created_at, users.updated_at FROM users`

func (s *SQLite) CreateUser(ctx context.Context, value auth.UserRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(
        id, name, email, password_hash, email_verified, verification_token_hash,
        verification_expires_at, created_at, updated_at
    ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.Name, value.Email, value.PasswordHash,
		value.EmailVerified, value.VerificationTokenHash, nullableTime(value.VerificationTokenExpiry),
		formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return auth.ErrEmailTaken
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *SQLite) DeleteUser(ctx context.Context, userID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return auth.ErrNotFound
	}
	return nil
}

func (s *SQLite) GetUserByID(ctx context.Context, userID string) (auth.UserRecord, error) {
	return scanUserRecord(s.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, userID))
}

func (s *SQLite) GetUserByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	return scanUserRecord(s.db.QueryRowContext(ctx, userSelect+` WHERE email = ?`, email))
}

func (s *SQLite) GetUserByVerificationToken(ctx context.Context, tokenHash string) (auth.UserRecord, error) {
	return scanUserRecord(s.db.QueryRowContext(ctx, userSelect+` WHERE verification_token_hash = ?`, tokenHash))
}

func (s *SQLite) MarkUserVerified(ctx context.Context, userID string, verifiedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET email_verified = 1,
        verification_token_hash = '', verification_expires_at = NULL, updated_at = ? WHERE id = ?`,
		formatTime(verifiedAt.UTC()), userID)
	if err != nil {
		return fmt.Errorf("verify user email: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return auth.ErrNotFound
	}
	return nil
}

func (s *SQLite) CreateSession(ctx context.Context, value auth.SessionRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, created_at, expires_at)
        VALUES(?, ?, ?, ?)`, value.TokenHash, value.UserID, formatTime(value.CreatedAt), formatTime(value.ExpiresAt))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *SQLite) GetUserBySession(ctx context.Context, tokenHash string, now time.Time) (auth.UserRecord, error) {
	value, err := scanUserRecord(s.db.QueryRowContext(ctx, userSelect+`
        JOIN sessions ON sessions.user_id = users.id
        WHERE sessions.token_hash = ? AND sessions.expires_at > ?`, tokenHash, formatTime(now.UTC())))
	if errors.Is(err, auth.ErrNotFound) {
		return auth.UserRecord{}, auth.ErrNotFound
	}
	return value, err
}

func (s *SQLite) DeleteSession(ctx context.Context, tokenHash string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return auth.ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, formatTime(now.UTC())); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

func (s *SQLite) ClaimUnownedTasks(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE tasks SET owner_id = ? WHERE owner_id = ''`, userID); err != nil {
		return fmt.Errorf("claim existing tasks: %w", err)
	}
	return nil
}

func scanUserRecord(row scanner) (auth.UserRecord, error) {
	var value auth.UserRecord
	var verified bool
	var verificationExpiry sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&value.ID, &value.Name, &value.Email, &value.PasswordHash, &verified,
		&value.VerificationTokenHash, &verificationExpiry, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.UserRecord{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.UserRecord{}, fmt.Errorf("scan user: %w", err)
	}
	value.EmailVerified = verified
	if verificationExpiry.Valid {
		parsed, err := parseTime(verificationExpiry.String)
		if err != nil {
			return auth.UserRecord{}, err
		}
		value.VerificationTokenExpiry = &parsed
	}
	var parseErr error
	if value.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return auth.UserRecord{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return auth.UserRecord{}, parseErr
	}
	return value, nil
}
