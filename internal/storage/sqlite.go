package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/id"
	"github.com/chuanye-gao/CronPilot/internal/task"
	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    schedule TEXT NOT NULL,
    timezone TEXT NOT NULL,
    prompt TEXT NOT NULL,
    enabled INTEGER NOT NULL,
    timeout_ns INTEGER NOT NULL,
    retry_max_attempts INTEGER NOT NULL,
    retry_delay_ns INTEGER NOT NULL,
    delivery_type TEXT NOT NULL,
    delivery_to TEXT NOT NULL,
    delivery_on TEXT NOT NULL,
    delivery_include_output INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_updated_at ON tasks(updated_at);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email_verified INTEGER NOT NULL DEFAULT 0,
    verification_token_hash TEXT NOT NULL DEFAULT '',
    verification_expires_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_users_verification_token ON users(verification_token_hash);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS executions (
    id TEXT PRIMARY KEY,
	owner_id TEXT NOT NULL DEFAULT '',
    task_id TEXT NOT NULL,
    task_name TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    output TEXT NOT NULL,
    error TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    delivery_status TEXT NOT NULL,
    delivery_error TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_executions_task_started ON executions(task_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_executions_started ON executions(started_at DESC);
`

const sqliteTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

type SQLite struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLite, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o750); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	query := make(url.Values)
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(NORMAL)")
	dsn := "file:" + filepath.ToSlash(absolutePath) + "?" + query.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &SQLite{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLite) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}
	return nil
}

func (s *SQLite) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	return nil
}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, sqliteSchema); err != nil {
		return fmt.Errorf("apply sqlite schema: %w", err)
	}
	hasOwner, err := sqliteColumnExists(ctx, tx, "tasks", "owner_id")
	if err != nil {
		return err
	}
	if !hasOwner {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add task owner column: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_tasks_owner_created ON tasks(owner_id, created_at)`); err != nil {
		return fmt.Errorf("create task owner index: %w", err)
	}
	hasExecutionOwner, err := sqliteColumnExists(ctx, tx, "executions", "owner_id")
	if err != nil {
		return err
	}
	if !hasExecutionOwner {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE executions ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add execution owner column: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE executions
		SET owner_id = COALESCE((SELECT owner_id FROM tasks WHERE tasks.id = executions.task_id), '')
		WHERE owner_id = ''`); err != nil {
		return fmt.Errorf("backfill execution owners: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_executions_owner_started ON executions(owner_id, started_at DESC)`); err != nil {
		return fmt.Errorf("create execution owner index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`, formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("record sqlite migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(2, ?)`, formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("record account migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(3, ?)`, formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("record execution owner migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite migration: %w", err)
	}
	return nil
}

func (s *SQLite) ListTasks(ctx context.Context) ([]task.Task, error) {
	rows, err := s.db.QueryContext(ctx, taskSelect+` ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list sqlite tasks: %w", err)
	}
	defer rows.Close()
	result := make([]task.Task, 0)
	for rows.Next() {
		value, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite tasks: %w", err)
	}
	return result, nil
}

func (s *SQLite) GetTask(ctx context.Context, taskID string) (task.Task, error) {
	value, err := scanTask(s.db.QueryRowContext(ctx, taskSelect+` WHERE id = ?`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, ErrNotFound
	}
	return value, err
}

func (s *SQLite) CreateTask(ctx context.Context, value task.Task) (task.Task, error) {
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
		return task.Task{}, fmt.Errorf("create sqlite task %q: %w", value.Name, err)
	}
	return value, nil
}

func (s *SQLite) UpdateTask(ctx context.Context, value task.Task) (task.Task, error) {
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
		return task.Task{}, fmt.Errorf("update sqlite task %q: %w", value.Name, err)
	}
	if err := requireAffected(result); err != nil {
		return task.Task{}, err
	}
	return value, nil
}

func (s *SQLite) DeleteTask(ctx context.Context, taskID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, taskID)
	if err != nil {
		return fmt.Errorf("delete sqlite task %q: %w", taskID, err)
	}
	return requireAffected(result)
}

func (s *SQLite) ListExecutions(ctx context.Context, taskID string, limit int) ([]execution.Execution, error) {
	query := executionSelect
	args := make([]any, 0, 2)
	if taskID != "" {
		query += ` WHERE task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY started_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sqlite executions: %w", err)
	}
	defer rows.Close()
	result := make([]execution.Execution, 0)
	for rows.Next() {
		value, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite executions: %w", err)
	}
	return result, nil
}

func (s *SQLite) ListExecutionsByOwner(ctx context.Context, ownerID string, limit int) ([]execution.Execution, error) {
	rows, err := s.db.QueryContext(ctx, executionSelect+` WHERE owner_id = ? ORDER BY started_at DESC, id DESC LIMIT ?`, ownerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list sqlite executions by owner: %w", err)
	}
	defer rows.Close()
	result := make([]execution.Execution, 0)
	for rows.Next() {
		value, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite executions by owner: %w", err)
	}
	return result, nil
}

func (s *SQLite) GetExecution(ctx context.Context, executionID string) (execution.Execution, error) {
	value, err := scanExecution(s.db.QueryRowContext(ctx, executionSelect+` WHERE id = ?`, executionID))
	if errors.Is(err, sql.ErrNoRows) {
		return execution.Execution{}, ErrNotFound
	}
	return value, err
}

func (s *SQLite) CreateExecution(ctx context.Context, value execution.Execution) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO executions(
		id, owner_id, task_id, task_name, status, started_at, finished_at, output, error, attempts, delivery_status, delivery_error
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OwnerID, value.TaskID, value.TaskName, value.Status, formatTime(value.StartedAt), nullableTime(value.FinishedAt),
		value.Output, value.Error, value.Attempts, value.DeliveryStatus, value.DeliveryError)
	if err != nil {
		return fmt.Errorf("create sqlite execution %q: %w", value.ID, err)
	}
	return nil
}

func (s *SQLite) UpdateExecution(ctx context.Context, value execution.Execution) error {
	result, err := s.db.ExecContext(ctx, `UPDATE executions SET
		owner_id = ?, task_id = ?, task_name = ?, status = ?, started_at = ?, finished_at = ?, output = ?, error = ?,
		attempts = ?, delivery_status = ?, delivery_error = ? WHERE id = ?`,
		value.OwnerID, value.TaskID, value.TaskName, value.Status, formatTime(value.StartedAt), nullableTime(value.FinishedAt), value.Output,
		value.Error, value.Attempts, value.DeliveryStatus, value.DeliveryError, value.ID)
	if err != nil {
		return fmt.Errorf("update sqlite execution %q: %w", value.ID, err)
	}
	return requireAffected(result)
}

func (s *SQLite) RecoverInterruptedExecutions(ctx context.Context, recoveredAt time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE executions
        SET status = ?, finished_at = ?, error = ?
        WHERE status IN (?, ?)`, execution.StatusInterrupted, formatTime(recoveredAt.UTC()),
		"CronPilot stopped before this execution finished", execution.StatusPending, execution.StatusRunning)
	if err != nil {
		return 0, fmt.Errorf("recover interrupted sqlite executions: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read recovered sqlite execution count: %w", err)
	}
	return count, nil
}

const taskSelect = `SELECT id, owner_id, name, description, schedule, timezone, prompt, enabled, timeout_ns,
    retry_max_attempts, retry_delay_ns, delivery_type, delivery_to, delivery_on,
    delivery_include_output, created_at, updated_at FROM tasks`

const executionSelect = `SELECT id, owner_id, task_id, task_name, status, started_at, finished_at, output, error,
	attempts, delivery_status, delivery_error FROM executions`

type scanner interface {
	Scan(...any) error
}

func scanTask(row scanner) (task.Task, error) {
	var value task.Task
	var enabled bool
	var timeout, retryDelay int64
	var deliveryTo, deliveryOn string
	var includeOutput sql.NullBool
	var createdAt, updatedAt string
	err := row.Scan(&value.ID, &value.OwnerID, &value.Name, &value.Description, &value.Schedule, &value.Timezone, &value.Prompt,
		&enabled, &timeout, &value.Retry.MaxAttempts, &retryDelay, &value.Delivery.Type, &deliveryTo, &deliveryOn,
		&includeOutput, &createdAt, &updatedAt)
	if err != nil {
		return task.Task{}, err
	}
	value.Enabled = &enabled
	value.Timeout = task.Duration(timeout)
	value.Retry.Delay = task.Duration(retryDelay)
	if err := json.Unmarshal([]byte(deliveryTo), &value.Delivery.To); err != nil {
		return task.Task{}, fmt.Errorf("decode task delivery recipients: %w", err)
	}
	if err := json.Unmarshal([]byte(deliveryOn), &value.Delivery.On); err != nil {
		return task.Task{}, fmt.Errorf("decode task delivery events: %w", err)
	}
	if includeOutput.Valid {
		value.Delivery.IncludeOutput = &includeOutput.Bool
	}
	var parseErr error
	if value.CreatedAt, parseErr = parseTime(createdAt); parseErr != nil {
		return task.Task{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt); parseErr != nil {
		return task.Task{}, parseErr
	}
	return value, nil
}

func scanExecution(row scanner) (execution.Execution, error) {
	var value execution.Execution
	var startedAt string
	var finishedAt sql.NullString
	err := row.Scan(&value.ID, &value.OwnerID, &value.TaskID, &value.TaskName, &value.Status, &startedAt, &finishedAt,
		&value.Output, &value.Error, &value.Attempts, &value.DeliveryStatus, &value.DeliveryError)
	if err != nil {
		return execution.Execution{}, err
	}
	var parseErr error
	if value.StartedAt, parseErr = parseTime(startedAt); parseErr != nil {
		return execution.Execution{}, parseErr
	}
	if finishedAt.Valid {
		parsed, err := parseTime(finishedAt.String)
		if err != nil {
			return execution.Execution{}, err
		}
		value.FinishedAt = &parsed
	}
	return value, nil
}

func marshalDelivery(value task.Delivery) (string, string, error) {
	to, err := json.Marshal(value.To)
	if err != nil {
		return "", "", fmt.Errorf("encode task delivery recipients: %w", err)
	}
	on, err := json.Marshal(value.On)
	if err != nil {
		return "", "", fmt.Errorf("encode task delivery events: %w", err)
	}
	return string(to), string(on), nil
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(value.UTC())
}

func formatTime(value time.Time) string {
	return value.UTC().Format(sqliteTimeFormat)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(sqliteTimeFormat, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, value)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("parse sqlite time %q: %w", value, err)
	}
	return parsed, nil
}

func requireAffected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected sqlite rows: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func sqliteColumnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect sqlite table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("inspect sqlite column: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite columns: %w", err)
	}
	return false, nil
}
