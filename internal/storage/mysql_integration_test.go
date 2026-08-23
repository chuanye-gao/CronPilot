package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/auth"
	"github.com/chuanye-gao/CronPilot/internal/execution"
	"github.com/chuanye-gao/CronPilot/internal/id"
	"github.com/chuanye-gao/CronPilot/internal/task"
)

func TestMySQLPersistence(t *testing.T) {
	if os.Getenv("CRONPILOT_MYSQL_TEST") != "1" {
		t.Skip("set CRONPILOT_MYSQL_TEST=1 to run the MySQL integration test")
	}
	store, err := NewMySQL(MySQLConfig{
		Address: os.Getenv("MYSQL_ADDRESS"), Username: os.Getenv("MYSQL_USERNAME"),
		Password: os.Getenv("MYSQL_PASSWORD"), Database: os.Getenv("MYSQL_DATABASE"),
	})
	if err != nil {
		t.Fatalf("NewMySQL() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := id.New("ci")
	now := time.Now().UTC().Truncate(time.Millisecond)
	verificationExpiry := now.Add(30 * time.Minute)
	user := auth.UserRecord{
		User:         auth.User{ID: "user_" + suffix, Name: "CI User", Email: fmt.Sprintf("%s@example.com", suffix), CreatedAt: now, UpdatedAt: now},
		PasswordHash: "test-hash", VerificationTokenHash: "verify_" + suffix, VerificationTokenExpiry: &verificationExpiry,
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteUser(context.Background(), user.ID) })
	if err := store.CreateUser(ctx, user); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("duplicate CreateUser() error = %v", err)
	}
	if err := store.MarkUserVerified(ctx, user.ID, now); err != nil {
		t.Fatalf("MarkUserVerified() error = %v", err)
	}
	session := auth.SessionRecord{TokenHash: "session_" + suffix, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := store.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	loadedUser, err := store.GetUserBySession(ctx, session.TokenHash, now)
	if err != nil || loadedUser.ID != user.ID || !loadedUser.EmailVerified {
		t.Fatalf("GetUserBySession() = %#v, %v", loadedUser, err)
	}

	enabled := true
	includeOutput := true
	createdTask, err := store.CreateTask(ctx, task.Task{
		OwnerID: user.ID, Name: "MySQL CI task", Description: "integration test", Schedule: "0 8 * * *", Timezone: "Asia/Shanghai",
		Prompt: "Write a brief", Enabled: &enabled, Timeout: task.Duration(time.Minute),
		Retry:    task.Retry{MaxAttempts: 2, Delay: task.Duration(time.Second)},
		Delivery: task.Delivery{Type: "email", To: []string{user.Email}, On: []string{"success"}, IncludeOutput: &includeOutput},
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteTask(context.Background(), createdTask.ID) })
	loadedTask, err := store.GetTask(ctx, createdTask.ID)
	if err != nil || loadedTask.OwnerID != user.ID || len(loadedTask.Delivery.To) != 1 {
		t.Fatalf("GetTask() = %#v, %v", loadedTask, err)
	}

	run := execution.New(createdTask)
	if err := store.CreateExecution(ctx, run); err != nil {
		t.Fatalf("CreateExecution() error = %v", err)
	}
	finishedAt := now.Add(time.Second)
	run.Status = execution.StatusSuccess
	run.FinishedAt = &finishedAt
	run.Output = "mysql integration passed"
	if err := store.UpdateExecution(ctx, run); err != nil {
		t.Fatalf("UpdateExecution() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() before reopen error = %v", err)
	}
	store, err = NewMySQL(MySQLConfig{
		Address: os.Getenv("MYSQL_ADDRESS"), Username: os.Getenv("MYSQL_USERNAME"),
		Password: os.Getenv("MYSQL_PASSWORD"), Database: os.Getenv("MYSQL_DATABASE"),
	})
	if err != nil {
		t.Fatalf("reopen MySQL error = %v", err)
	}
	loadedTask, err = store.GetTask(ctx, createdTask.ID)
	if err != nil || loadedTask.ID != createdTask.ID {
		t.Fatalf("persisted task after reopen = %#v, %v", loadedTask, err)
	}
	runs, err := store.ListExecutionsByOwner(ctx, user.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].Output != run.Output {
		t.Fatalf("ListExecutionsByOwner() = %#v, %v", runs, err)
	}
}
