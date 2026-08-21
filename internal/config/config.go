package config

import (
	"fmt"
	"os"

	"github.com/chuanye-gao/CronPilot/internal/task"
	"gopkg.in/yaml.v3"
)

type LLMConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type Config struct {
	Timezone string      `yaml:"timezone"`
	LLM      LLMConfig   `yaml:"llm"`
	Tasks    []task.Task `yaml:"tasks"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.LLM.Model == "" {
		return Config{}, fmt.Errorf("llm.model is required")
	}
	if cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = os.Getenv("CRONPILOT_API_KEY")
	}
	if cfg.LLM.APIKey == "" {
		return Config{}, fmt.Errorf("llm.api_key or CRONPILOT_API_KEY is required")
	}

	for i := range cfg.Tasks {
		if err := cfg.Tasks[i].Validate(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}
