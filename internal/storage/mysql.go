package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/id"
	"github.com/chuanye-gao/CronPilot/internal/task"
	driver "github.com/go-sql-driver/mysql"
)

type MySQLConfig struct {
	Address  string
	Username string
	Password string
	Database string
}

type MySQL struct {
	db *sql.DB
}

var mysqlSchema = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT NOT NULL PRIMARY KEY,
		applied_at VARCHAR(40) NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS tasks (
		id VARCHAR(128) NOT NULL PRIMARY KEY,
		owner_id VARCHAR(128) NOT NULL DEFAULT '',
		name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL,
		schedule VARCHAR(128) NOT NULL,
		timezone VARCHAR(128) NOT NULL,
		prompt LONGTEXT NOT NULL,
		enabled TINYINT(1) NOT NULL,
		timeout_ns BIGINT NOT NULL,
		retry_max_attempts INT NOT NULL,
		retry_delay_ns BIGINT NOT NULL,
		delivery_type VARCHAR(64) NOT NULL,
		delivery_to TEXT NOT NULL,
		delivery_on TEXT NOT NULL,
		delivery_include_output TINYINT(1) NULL,
		created_at VARCHAR(40) NOT NULL,
		updated_at VARCHAR(40) NOT NULL,
		INDEX idx_tasks_updated_at (updated_at),
		INDEX idx_tasks_owner_created (owner_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(128) NOT NULL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(254) NOT NULL UNIQUE,
		password_hash VARCHAR(512) NOT NULL,
		email_verified TINYINT(1) NOT NULL DEFAULT 0,
		verification_token_hash VARCHAR(128) NOT NULL DEFAULT '',
		verification_expires_at VARCHAR(40) NULL,
		created_at VARCHAR(40) NOT NULL,
		updated_at VARCHAR(40) NOT NULL,
		INDEX idx_users_verification_token (verification_token_hash)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS sessions (
		token_hash VARCHAR(128) NOT NULL PRIMARY KEY,
		user_id VARCHAR(128) NOT NULL,
		created_at VARCHAR(40) NOT NULL,
		expires_at VARCHAR(40) NOT NULL,
		INDEX idx_sessions_user (user_id),
		INDEX idx_sessions_expiry (expires_at),
		CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	`CREATE TABLE IF NOT EXISTS executions (
		id VARCHAR(128) NOT NULL PRIMARY KEY,
		owner_id VARCHAR(128) NOT NULL DEFAULT '',
		task_id VARCHAR(128) NOT NULL,
		task_name VARCHAR(255) NOT NULL,
		status VARCHAR(64) NOT NULL,
		started_at VARCHAR(40) NOT NULL,
		finished_at VARCHAR(40) NULL,
		output LONGTEXT NOT NULL,
		error LONGTEXT NOT NULL,
		attempts INT NOT NULL,
		delivery_status VARCHAR(64) NOT NULL,
		delivery_error LONGTEXT NOT NULL,
		INDEX idx_executions_task_started (task_id, started_at),
		INDEX idx_executions_started (started_at),
		INDEX idx_executions_owner_started (owner_id, started_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
}

func NewMySQL(cfg MySQLConfig) (*MySQL, error) {
	if strings.TrimSpace(cfg.Address) == "" || strings.TrimSpace(cfg.Username) == "" || cfg.Password == "" || strings.TrimSpace(cfg.Database) == "" {
		return nil, fmt.Errorf("mysql address, username, password, and database are required")
	}
	driverConfig := driver.NewConfig()
	driverConfig.Net = "tcp"
	driverConfig.Addr = cfg.Address
	driverConfig.User = cfg.Username
	driverConfig.Passwd = cfg.Password
	driverConfig.DBName = cfg.Database
	driverConfig.Timeout = 10 * time.Second
	driverConfig.ReadTimeout = 35 * time.Second
	driverConfig.WriteTimeout = 35 * time.Second
	driverConfig.InterpolateParams = true
	driverConfig.Params = map[string]string{"charset": "utf8mb4", "collation": "utf8mb4_unicode_ci"}

	db, err := openMySQL(driverConfig)
	if err != nil {
		var mysqlErr *driver.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1049 {
			return nil, err
		}
		serverConfig := *driverConfig
		serverConfig.DBName = ""
		server, openErr := openMySQL(&serverConfig)
		if openErr != nil {
			return nil, openErr
		}
		_, createErr := server.Exec(`CREATE DATABASE IF NOT EXISTS ` + quoteMySQLIdentifier(cfg.Database) + ` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`)
		_ = server.Close()
		if createErr != nil {
			return nil, fmt.Errorf("create mysql database %q: %w", cfg.Database, createErr)
		}
		db, err = openMySQL(driverConfig)
		if err != nil {
			return nil, err
		}
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	store := &MySQL{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func openMySQL(cfg *driver.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql database: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to mysql database: %w", err)
	}
	return db, nil
}

func quoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func (s *MySQL) migrate(ctx context.Context) error {
	for _, statement := range mysqlSchema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply mysql schema: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?), (2, ?), (3, ?), (4, ?)`,
		formatTime(time.Now().UTC()), formatTime(time.Now().UTC()), formatTime(time.Now().UTC()), formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("record mysql migrations: %w", err)
	}
	return nil
}

func (s *MySQL) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close mysql database: %w", err)
	}
	return nil
}

func (s *MySQL) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql database: %w", err)
	}
	return nil
}

func (s *MySQL) ListTasks(ctx context.Context) ([]task.Task, error) {
	rows, err := s.db.QueryContext(ctx, taskSelect+` ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list mysql tasks: %w", err)
	}
	defer rows.Close()
	result := make([]task.Task, 0)
	for rows.Next() {
		value, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mysql tasks: %w", err)
	}
	return result, nil
}

func (s *MySQL) GetTask(ctx context.Context, taskID string) (task.Task, error) {
	value, err := scanTask(s.db.QueryRowContext(ctx, taskSelect+` WHERE id = ?`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, ErrNotFound
	}
	return value, err
}

func (s *MySQL) CreateTask(ctx context.Context, value task.Task) (task.Task, error) {
	if value.ID == "" {
		value.ID = id.New("task")
	}
	now := time.Now().UTC()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	to, on, err := marshalDelivery(value.Delivery)
	if err != nil {
		return task.Task{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO tasks(
		id, owner_id, name, description, schedule, timezone, prompt, enabled, timeout_ns,
		retry_max_attempts, retry_delay_ns, delivery_type, delivery_to, delivery_on,
		delivery_include_output, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OwnerID, value.Name, value.Description, value.Schedule, value.Timezone, value.Prompt, value.IsEnabled(), int64(value.Timeout),
		value.Retry.MaxAttempts, int64(value.Retry.Delay), value.Delivery.Type, to, on, nullableBool(value.Delivery.IncludeOutput),
		formatTime(value.CreatedAt), formatTime(value.UpdatedAt))
	if err != nil {
		return task.Task{}, fmt.Errorf("create mysql task %q: %w", value.Name, err)
	}
	return value, nil
}

func (s *MySQL) UpdateTask(ctx context.Context, value task.Task) (task.Task, error) {
	current, err := s.GetTask(ctx, value.ID)
	if err != nil {
		return task.Task{}, err
	}
	value.CreatedAt = current.CreatedAt
	if value.OwnerID == "" {
		value.OwnerID = current.OwnerID
	}
	value.UpdatedAt = time.Now().UTC()
	to, on, err := marshalDelivery(value.Delivery)
	if err != nil {
		return task.Task{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET
		owner_id = ?, name = ?, description = ?, schedule = ?, timezone = ?, prompt = ?, enabled = ?, timeout_ns = ?,
		retry_max_attempts = ?, retry_delay_ns = ?, delivery_type = ?, delivery_to = ?, delivery_on = ?,
		delivery_include_output = ?, updated_at = ? WHERE id = ?`,
		value.OwnerID, value.Name, value.Description, value.Schedule, value.Timezone, value.Prompt, value.IsEnabled(), int64(value.Timeout),
		value.Retry.MaxAttempts, int64(value.Retry.Delay), value.Delivery.Type, to, on,
		nullableBool(value.Delivery.IncludeOutput), formatTime(value.UpdatedAt), value.ID)
	if err != nil {
		return task.Task{}, fmt.Errorf("update mysql task %q: %w", value.Name, err)
	}
	if err := requireAffected(result); err != nil {
		return task.Task{}, err
	}
	return value, nil
}

func (s *MySQL) DeleteTask(ctx context.Context, taskID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, taskID)
	if err != nil {
		return fmt.Errorf("delete mysql task %q: %w", taskID, err)
	}
	return requireAffected(result)
}

func (s *MySQL) ListExecutions(ctx context.Context, taskID string, limit int) ([]execution.Execution, error) {
	query := executionSelect
	args := make([]any, 0, 2)
	if taskID != "" {
		query += ` WHERE task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY started_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	return s.listExecutions(ctx, query, args...)
}

func (s *MySQL) ListExecutionsByOwner(ctx context.Context, ownerID string, limit int) ([]execution.Execution, error) {
	return s.listExecutions(ctx, executionSelect+` WHERE owner_id = ? ORDER BY started_at DESC, id DESC LIMIT ?`, ownerID, limit)
}

func (s *MySQL) listExecutions(ctx context.Context, query string, args ...any) ([]execution.Execution, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list mysql executions: %w", err)
	}
	defer rows.Close()
	result := make([]execution.Execution, 0)
	for rows.Next() {
		value, scanErr := scanExecution(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mysql executions: %w", err)
	}
	return result, nil
}

func (s *MySQL) GetExecution(ctx context.Context, executionID string) (execution.Execution, error) {
	value, err := scanExecution(s.db.QueryRowContext(ctx, executionSelect+` WHERE id = ?`, executionID))
	if errors.Is(err, sql.ErrNoRows) {
		return execution.Execution{}, ErrNotFound
	}
	return value, err
}

func (s *MySQL) CreateExecution(ctx context.Context, value execution.Execution) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO executions(
		id, owner_id, task_id, task_name, status, started_at, finished_at, output, error, attempts, delivery_status, delivery_error
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OwnerID, value.TaskID, value.TaskName, value.Status, formatTime(value.StartedAt), nullableTime(value.FinishedAt),
		value.Output, value.Error, value.Attempts, value.DeliveryStatus, value.DeliveryError)
	if err != nil {
		return fmt.Errorf("create mysql execution %q: %w", value.ID, err)
	}
	return nil
}

func (s *MySQL) UpdateExecution(ctx context.Context, value execution.Execution) error {
	result, err := s.db.ExecContext(ctx, `UPDATE executions SET
		owner_id = ?, task_id = ?, task_name = ?, status = ?, started_at = ?, finished_at = ?, output = ?, error = ?,
		attempts = ?, delivery_status = ?, delivery_error = ? WHERE id = ?`,
		value.OwnerID, value.TaskID, value.TaskName, value.Status, formatTime(value.StartedAt), nullableTime(value.FinishedAt), value.Output,
		value.Error, value.Attempts, value.DeliveryStatus, value.DeliveryError, value.ID)
	if err != nil {
		return fmt.Errorf("update mysql execution %q: %w", value.ID, err)
	}
	return requireAffected(result)
}

func (s *MySQL) RecoverInterruptedExecutions(ctx context.Context, recoveredAt time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE executions
		SET status = ?, finished_at = ?, error = ?
		WHERE status IN (?, ?)`, execution.StatusInterrupted, formatTime(recoveredAt.UTC()),
		"CronPilot stopped before this execution finished", execution.StatusPending, execution.StatusRunning)
	if err != nil {
		return 0, fmt.Errorf("recover interrupted mysql executions: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read recovered mysql execution count: %w", err)
	}
	return count, nil
}
