package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/api"
	"github.com/chuanye-gao/CronPilot/internal/auth"
	"github.com/chuanye-gao/CronPilot/internal/config"
	"github.com/chuanye-gao/CronPilot/internal/delivery"
	"github.com/chuanye-gao/CronPilot/internal/llm"
	"github.com/chuanye-gao/CronPilot/internal/runner"
	"github.com/chuanye-gao/CronPilot/internal/scheduler"
	"github.com/chuanye-gao/CronPilot/internal/storage"
	"github.com/chuanye-gao/CronPilot/internal/websearch"
)

type applicationStore interface {
	storage.Store
	storage.RecoveryStore
	auth.Store
	Close() error
}

func main() {
	configPath := flag.String("config", "cronpilot.yaml", "path to CronPilot config file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	logger = newLogger(cfg.Log)

	location := time.Local
	if cfg.Timezone != "" {
		location, err = time.LoadLocation(cfg.Timezone)
		if err != nil {
			logger.Error("load timezone", "timezone", cfg.Timezone, "error", err)
			os.Exit(1)
		}
	}

	var searchAgent *websearch.Agent
	llmOptions := make([]llm.OpenAIOption, 0, 4)
	if cfg.WebSearch.Enabled {
		searchAgent, err = websearch.New(websearch.Config{
			Provider: cfg.WebSearch.Provider,
			Endpoint: cfg.WebSearch.Endpoint, Timeout: time.Duration(cfg.WebSearch.Timeout),
			APIKey:     cfg.WebSearch.APIKey,
			MaxResults: cfg.WebSearch.MaxResults, MaxContentChars: cfg.WebSearch.MaxContentChars,
		}, logger)
		if err != nil {
			logger.Error("configure web search", "error", err)
			os.Exit(1)
		}
		llmOptions = append(llmOptions,
			llm.WithTools(searchAgent.Tools()...),
			llm.WithSystemPrompt(websearch.SystemPrompt(location)),
			llm.WithMaxToolRounds(cfg.WebSearch.MaxToolRounds),
			llm.WithToolObserver(func(event llm.ToolEvent) {
				if event.Error != "" {
					logger.Warn("model tool call failed", "tool", event.Name, "duration", event.Duration, "error", event.Error)
					return
				}
				logger.Info("model tool call completed", "tool", event.Name, "duration", event.Duration)
			}),
		)
		logger.Info("web search enabled", "provider", searchAgent.Provider(), "endpoint", cfg.WebSearch.Endpoint)
	}
	primaryClient := llm.NewOpenAIClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model, llmOptions...)
	primaryAssistant := llm.NewOpenAIClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)
	var client llm.Client = primaryClient
	var assistantClient llm.Client = primaryAssistant
	var geminiAssistant llm.Client
	if cfg.Gemini.APIKey != "" {
		geminiClient := llm.NewOpenAIClient(cfg.Gemini.BaseURL, cfg.Gemini.APIKey, cfg.Gemini.Model, llmOptions...)
		geminiAssistant = llm.NewOpenAIClient(cfg.Gemini.BaseURL, cfg.Gemini.APIKey, cfg.Gemini.Model)
		client = llm.NewFallbackClient(primaryClient, geminiClient, func(reason string) {
			logger.Warn("switching task execution to fallback model", "reason", reason, "fallback_model", cfg.Gemini.Model)
		})
		assistantClient = llm.NewFallbackClient(primaryAssistant, geminiAssistant, func(reason string) {
			logger.Warn("switching task assistant to fallback model", "reason", reason, "fallback_model", cfg.Gemini.Model)
		})
		logger.Info("fallback model enabled", "model", cfg.Gemini.Model)
	}
	emailDelivery := delivery.NewEmail(nil, "")
	if cfg.Email.Host != "" {
		smtpSender, smtpErr := delivery.NewSMTP(delivery.SMTPConfig{
			Host:     cfg.Email.Host,
			Port:     cfg.Email.Port,
			Username: cfg.Email.Username,
			Password: cfg.Email.Password,
			TLS:      cfg.Email.TLS,
			Timeout:  time.Duration(cfg.Email.Timeout),
		})
		if smtpErr != nil {
			logger.Error("configure email delivery", "error", smtpErr)
			os.Exit(1)
		}
		emailDelivery = delivery.NewEmail(smtpSender, cfg.Email.From)
	}
	var appStore applicationStore
	switch cfg.Database.Driver {
	case "mysql":
		appStore, err = storage.NewMySQL(storage.MySQLConfig{
			Address: cfg.Database.Address, Username: cfg.Database.Username,
			Password: cfg.Database.Password, Database: cfg.Database.Name,
		})
	case "sqlite":
		appStore, err = storage.NewSQLite(cfg.Database.Path)
	default:
		err = fmt.Errorf("unsupported database driver %q", cfg.Database.Driver)
	}
	if err != nil {
		logger.Error("open database", "driver", cfg.Database.Driver, "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := appStore.Close(); closeErr != nil {
			logger.Error("close database", "error", closeErr)
		}
	}()
	accountService, err := auth.NewService(appStore, emailDelivery, cfg.Server.PublicURL)
	if err != nil {
		logger.Error("configure account service", "error", err)
		os.Exit(1)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()
	recovered, err := appStore.RecoverInterruptedExecutions(startupCtx, time.Now().UTC())
	if err != nil {
		logger.Error("recover interrupted executions", "error", err)
		os.Exit(1)
	}
	if recovered > 0 {
		logger.Warn("recovered interrupted executions", "count", recovered)
	}

	persistedTasks, err := appStore.ListTasks(startupCtx)
	if err != nil {
		logger.Error("load persisted tasks", "error", err)
		os.Exit(1)
	}
	if len(persistedTasks) == 0 && len(cfg.Tasks) > 0 {
		for _, configuredTask := range cfg.Tasks {
			createdTask, createErr := appStore.CreateTask(startupCtx, configuredTask)
			if createErr != nil {
				logger.Error("seed configured task", "task", configuredTask.Name, "error", createErr)
				os.Exit(1)
			}
			persistedTasks = append(persistedTasks, createdTask)
		}
		logger.Info("seeded database from configuration", "tasks", len(persistedTasks))
	} else if len(persistedTasks) > 0 && len(cfg.Tasks) > 0 {
		logger.Info("using persisted tasks; configuration tasks are only imported into an empty database", "persisted_tasks", len(persistedTasks))
	}

	taskRunner := runner.New(client, appStore, logger, runner.WithDelivery(emailDelivery))
	cronScheduler := scheduler.New(location, taskRunner, logger)

	for _, persistedTask := range persistedTasks {
		if err := cronScheduler.Add(persistedTask); err != nil {
			logger.Error("register task", "task", persistedTask.Name, "error", err)
			os.Exit(1)
		}
	}

	cronScheduler.Start()
	integrationChecks := map[string]api.IntegrationCheck{
		"database": func(ctx context.Context) error { return appStore.Ping(ctx) },
		"deepseek": func(ctx context.Context) error {
			output, checkErr := primaryAssistant.Complete(ctx, "Reply with exactly: OK")
			if checkErr != nil {
				return checkErr
			}
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("model returned an empty response")
			}
			return nil
		},
	}
	if geminiAssistant != nil {
		integrationChecks["gemini"] = func(ctx context.Context) error {
			output, checkErr := geminiAssistant.Complete(ctx, "Reply with exactly: OK")
			if checkErr != nil {
				return checkErr
			}
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("model returned an empty response")
			}
			return nil
		}
	}
	if searchAgent != nil {
		integrationChecks["tavily"] = func(ctx context.Context) error {
			_, searchErr := searchAgent.Search(ctx, websearch.SearchRequest{
				Query: "latest artificial intelligence news", Category: "news", TimeRange: "day", Language: "en-US", MaxResults: 1,
			})
			return searchErr
		}
	}

	httpServer := &http.Server{
		Addr: cfg.Server.Address,
		Handler: api.New(api.Options{
			Store:               appStore,
			Runner:              taskRunner,
			Scheduler:           cronScheduler,
			Logger:              logger,
			DefaultTimezone:     location.String(),
			Model:               cfg.LLM.Model,
			FallbackModel:       cfg.Gemini.Model,
			FallbackConfigured:  cfg.Gemini.APIKey != "",
			ProviderConfigured:  cfg.LLM.APIKey != "",
			Email:               emailDelivery,
			Assistant:           assistantClient,
			TaskExecutor:        client,
			Auth:                accountService,
			Storage:             cfg.Database.Driver,
			WebSearchConfigured: searchAgent != nil,
			WebSearchProvider: func() string {
				if searchAgent == nil {
					return ""
				}
				return searchAgent.Provider()
			}(),
			WebSearchHealth: func(ctx context.Context) error {
				if searchAgent == nil {
					return nil
				}
				return searchAgent.Health(ctx)
			},
			RelayConfigured:   cfg.Relay.URL != "",
			IntegrationChecks: integrationChecks,
			Readiness: func(ctx context.Context) error {
				if err := appStore.Ping(ctx); err != nil {
					return err
				}
				if !cronScheduler.Ready() {
					return fmt.Errorf("scheduler is not ready")
				}
				return nil
			},
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       35 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("web console started", "address", "http://"+cfg.Server.Address)
		serverErr <- httpServer.ListenAndServe()
	}()

	logger.Info("CronPilot started", "tasks", len(persistedTasks), "timezone", location.String(), "database", cfg.Database.Driver)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("web console", "error", err)
		}
	}

	schedulerCtx := cronScheduler.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("stop web console", "error", err)
	}
	if err := taskRunner.Shutdown(shutdownCtx); err != nil {
		logger.Error("stop task runner", "error", err)
	}
	select {
	case <-schedulerCtx.Done():
	case <-shutdownCtx.Done():
		logger.Error("stop scheduler", "error", shutdownCtx.Err())
	}
	logger.Info("CronPilot stopped")
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	level := new(slog.LevelVar)
	switch cfg.Level {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
	options := &slog.HandlerOptions{Level: level}
	if cfg.Format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, options))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, options))
}
