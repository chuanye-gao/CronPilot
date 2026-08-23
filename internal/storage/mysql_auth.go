package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/auth"
	driver "github.com/go-sql-driver/mysql"
)

func (s *MySQL) CreateUser(ctx context.Context, value auth.UserRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(
		id, name, email, password_hash, email_verified, verification_token_hash,
		verification_expires_at, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.Name, value.Email, value.PasswordHash,
		value.EmailVerified, value.VerificationTokenHash, nullableTime(value.VerificationTokenExpiry),
		formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil {
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return auth.ErrEmailTaken
		}
		return fmt.Errorf("create mysql user: %w", err)
	}
	return nil
}

func (s *MySQL) DeleteUser(ctx context.Context, userID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete mysql user: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return auth.ErrNotFound
	}
	return nil
}

func (s *MySQL) GetUserByID(ctx context.Context, userID string) (auth.UserRecord, error) {
	return scanUserRecord(s.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, userID))
}

func (s *MySQL) GetUserByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	return scanUserRecord(s.db.QueryRowContext(ctx, userSelect+` WHERE email = ?`, email))
}

func (s *MySQL) GetUserByVerificationToken(ctx context.Context, tokenHash string) (auth.UserRecord, error) {
	return scanUserRecord(s.db.QueryRowContext(ctx, userSelect+` WHERE verification_token_hash = ?`, tokenHash))
}

func (s *MySQL) MarkUserVerified(ctx context.Context, userID string, verifiedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET email_verified = 1,
		verification_token_hash = '', verification_expires_at = NULL, updated_at = ? WHERE id = ?`,
		formatTime(verifiedAt.UTC()), userID)
	if err != nil {
		return fmt.Errorf("verify mysql user email: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return auth.ErrNotFound
	}
	return nil
}

func (s *MySQL) CreateSession(ctx context.Context, value auth.SessionRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, created_at, expires_at)
		VALUES(?, ?, ?, ?)`, value.TokenHash, value.UserID, formatTime(value.CreatedAt), formatTime(value.ExpiresAt))
	if err != nil {
		return fmt.Errorf("create mysql session: %w", err)
	}
	return nil
}

func (s *MySQL) GetUserBySession(ctx context.Context, tokenHash string, now time.Time) (auth.UserRecord, error) {
	value, err := scanUserRecord(s.db.QueryRowContext(ctx, userSelect+`
		JOIN sessions ON sessions.user_id = users.id
		WHERE sessions.token_hash = ? AND sessions.expires_at > ?`, tokenHash, formatTime(now.UTC())))
	if errors.Is(err, auth.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return auth.UserRecord{}, auth.ErrNotFound
	}
	return value, err
}

func (s *MySQL) DeleteSession(ctx context.Context, tokenHash string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete mysql session: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return auth.ErrNotFound
	}
	return nil
}

func (s *MySQL) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, formatTime(now.UTC())); err != nil {
		return fmt.Errorf("delete expired mysql sessions: %w", err)
	}
	return nil
}

func (s *MySQL) ClaimUnownedTasks(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE tasks SET owner_id = ? WHERE owner_id = ''`, userID); err != nil {
		return fmt.Errorf("claim existing mysql tasks: %w", err)
	}
	return nil
}
