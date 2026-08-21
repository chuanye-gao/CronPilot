package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/config"
	"github.com/chuanye-gao/CronPilot/internal/llm"
	"github.com/chuanye-gao/CronPilot/internal/runner"
	"github.com/chuanye-gao/CronPilot/internal/scheduler"
)

func main() {
	configPath := flag.String("config", "cronpilot.yaml", "path to CronPilot config file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	location := time.Local
	if cfg.Timezone != "" {
		location, err = time.LoadLocation(cfg.Timezone)
		if err != nil {
			logger.Error("load timezone", "timezone", cfg.Timezone, "error", err)
			os.Exit(1)
		}
	}

	client := llm.NewOpenAIClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)
	taskRunner := runner.New(client, logger)
	cronScheduler := scheduler.New(location, taskRunner, logger)

	for _, t := range cfg.Tasks {
		if err := cronScheduler.Add(t); err != nil {
			logger.Error("register task", "task", t.Name, "error", err)
			os.Exit(1)
		}
	}

	cronScheduler.Start()
	logger.Info("CronPilot started", "tasks", len(cfg.Tasks), "timezone", location.String())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx := cronScheduler.Stop()
	<-ctx.Done()
	logger.Info("CronPilot stopped")
}
