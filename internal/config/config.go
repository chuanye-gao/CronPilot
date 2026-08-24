package config

import (
	"fmt"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/chuanye-gao/CronPilot/internal/task"
	"gopkg.in/yaml.v3"
)

type LLMConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type WebSearchConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Provider        string        `yaml:"provider"`
	Endpoint        string        `yaml:"endpoint"`
	APIKey          string        `yaml:"api_key"`
	APIKeyEnv       string        `yaml:"api_key_env"`
	Timeout         task.Duration `yaml:"timeout"`
	MaxResults      int           `yaml:"max_results"`
	MaxContentChars int           `yaml:"max_content_chars"`
	MaxToolRounds   int           `yaml:"max_tool_rounds"`
}

type ServerConfig struct {
	Address   string `yaml:"address"`
	PublicURL string `yaml:"public_url"`
}

type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Path     string `yaml:"path"`
	Address  string `yaml:"address"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type LogConfig struct {
	Format string `yaml:"format"`
	Level  string `yaml:"level"`
}

type EmailConfig struct {
	Host        string        `yaml:"host"`
	Port        int           `yaml:"port"`
	Username    string        `yaml:"username"`
	UsernameEnv string        `yaml:"username_env"`
	Password    string        `yaml:"password"`
	PasswordEnv string        `yaml:"password_env"`
	From        string        `yaml:"from"`
	TLS         string        `yaml:"tls"`
	Timeout     task.Duration `yaml:"timeout"`
}

type Config struct {
	Timezone  string          `yaml:"timezone"`
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Log       LogConfig       `yaml:"log"`
	Email     EmailConfig     `yaml:"email"`
	LLM       LLMConfig       `yaml:"llm"`
	Gemini    LLMConfig       `yaml:"gemini"`
	WebSearch WebSearchConfig `yaml:"web_search"`
	Tasks     []task.Task     `yaml:"tasks"`
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
	if cfg.Server.Address == "" {
		cfg.Server.Address = "127.0.0.1:8080"
	}
	if value := strings.TrimSpace(os.Getenv("CRONPILOT_SERVER_ADDRESS")); value != "" {
		cfg.Server.Address = value
	}
	if cfg.Server.PublicURL == "" {
		cfg.Server.PublicURL = "http://127.0.0.1:8080"
	}
	if value := strings.TrimSpace(os.Getenv("CRONPILOT_PUBLIC_URL")); value != "" {
		cfg.Server.PublicURL = value
	}
	applyDatabaseEnvironment(&cfg.Database)
	if err := validateDatabase(&cfg.Database); err != nil {
		return Config{}, err
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "text"
	}
	if value := strings.TrimSpace(os.Getenv("CRONPILOT_LOG_FORMAT")); value != "" {
		cfg.Log.Format = value
	}
	cfg.Log.Format = strings.ToLower(strings.TrimSpace(cfg.Log.Format))
	if cfg.Log.Format != "text" && cfg.Log.Format != "json" {
		return Config{}, fmt.Errorf("log.format must be text or json")
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if value := strings.TrimSpace(os.Getenv("CRONPILOT_LOG_LEVEL")); value != "" {
		cfg.Log.Level = value
	}
	cfg.Log.Level = strings.ToLower(strings.TrimSpace(cfg.Log.Level))
	if cfg.Log.Level != "debug" && cfg.Log.Level != "info" && cfg.Log.Level != "warn" && cfg.Log.Level != "error" {
		return Config{}, fmt.Errorf("log.level must be debug, info, warn, or error")
	}
	if cfg.Email.Host != "" {
		if cfg.Email.Port == 0 {
			cfg.Email.Port = 465
		}
		if cfg.Email.TLS == "" {
			cfg.Email.TLS = "implicit"
		}
		if cfg.Email.Timeout == 0 {
			cfg.Email.Timeout = task.Duration(20 * time.Second)
		}
		if cfg.Email.UsernameEnv == "" {
			cfg.Email.UsernameEnv = "CRONPILOT_SMTP_USERNAME"
		}
		if cfg.Email.PasswordEnv == "" {
			cfg.Email.PasswordEnv = "CRONPILOT_SMTP_PASSWORD"
		}
		if cfg.Email.Username == "" {
			cfg.Email.Username = os.Getenv(cfg.Email.UsernameEnv)
		}
		if cfg.Email.Password == "" {
			cfg.Email.Password = os.Getenv(cfg.Email.PasswordEnv)
		}
		if cfg.Email.From == "" {
			cfg.Email.From = cfg.Email.Username
		}
		if _, err := mail.ParseAddress(cfg.Email.From); err != nil {
			return Config{}, fmt.Errorf("email.from is required and must be a valid address: %w", err)
		}
		if strings.ToLower(cfg.Email.TLS) != "none" && (cfg.Email.Username == "" || cfg.Email.Password == "") {
			return Config{}, fmt.Errorf("email SMTP credentials are required; set %s and %s", cfg.Email.UsernameEnv, cfg.Email.PasswordEnv)
		}
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
	if cfg.Gemini.BaseURL == "" {
		cfg.Gemini.BaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}
	if value := strings.TrimSpace(os.Getenv("GEMINI_BASE_URL")); value != "" {
		cfg.Gemini.BaseURL = value
	}
	if cfg.Gemini.Model == "" {
		cfg.Gemini.Model = "gemini-2.5-flash-lite"
	}
	if value := strings.TrimSpace(os.Getenv("GEMINI_MODEL")); value != "" {
		cfg.Gemini.Model = value
	}
	if cfg.Gemini.APIKey == "" {
		cfg.Gemini.APIKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	if cfg.WebSearch.APIKeyEnv == "" {
		cfg.WebSearch.APIKeyEnv = "TAVILY_API_KEY"
	}
	apiKeyFromEnvironment := strings.TrimSpace(os.Getenv(cfg.WebSearch.APIKeyEnv))
	if cfg.WebSearch.APIKey == "" {
		cfg.WebSearch.APIKey = apiKeyFromEnvironment
	}
	providerFromEnvironment := strings.ToLower(strings.TrimSpace(os.Getenv("CRONPILOT_WEB_SEARCH_PROVIDER")))
	endpointFromEnvironment := strings.TrimSpace(os.Getenv("CRONPILOT_WEB_SEARCH_ENDPOINT"))
	switch {
	case providerFromEnvironment != "":
		cfg.WebSearch.Provider = providerFromEnvironment
		cfg.WebSearch.Enabled = true
		if endpointFromEnvironment != "" {
			cfg.WebSearch.Endpoint = endpointFromEnvironment
		} else if providerFromEnvironment == "tavily" {
			cfg.WebSearch.Endpoint = "https://api.tavily.com"
		}
	case apiKeyFromEnvironment != "":
		// TAVILY_API_KEY alone always selects the official Tavily endpoint. This
		// deliberately ignores a stale SearXNG endpoint left in older deployments.
		cfg.WebSearch.Provider = "tavily"
		cfg.WebSearch.Enabled = true
		cfg.WebSearch.Endpoint = "https://api.tavily.com"
	case endpointFromEnvironment != "":
		// CRONPILOT_WEB_SEARCH_ENDPOINT was the legacy one-variable SearXNG
		// configuration. Preserve that behavior even though the current Docker
		// template names Tavily as its disabled provider.
		cfg.WebSearch.Provider = "searxng"
		cfg.WebSearch.Endpoint = endpointFromEnvironment
		cfg.WebSearch.Enabled = true
	}
	if cfg.WebSearch.Enabled {
		cfg.WebSearch.Provider = strings.ToLower(strings.TrimSpace(cfg.WebSearch.Provider))
		if cfg.WebSearch.Provider == "" {
			cfg.WebSearch.Provider = "searxng"
		}
		switch cfg.WebSearch.Provider {
		case "tavily":
			if cfg.WebSearch.APIKey == "" {
				return Config{}, fmt.Errorf("web_search.api_key or %s is required for Tavily", cfg.WebSearch.APIKeyEnv)
			}
			if strings.TrimSpace(cfg.WebSearch.Endpoint) == "" {
				cfg.WebSearch.Endpoint = "https://api.tavily.com"
			}
		case "searxng":
			if strings.TrimSpace(cfg.WebSearch.Endpoint) == "" {
				cfg.WebSearch.Endpoint = "http://127.0.0.1:8081"
			}
		default:
			return Config{}, fmt.Errorf("web_search.provider must be tavily or searxng")
		}
		if cfg.WebSearch.Timeout == 0 {
			cfg.WebSearch.Timeout = task.Duration(15 * time.Second)
		}
		if cfg.WebSearch.MaxResults == 0 {
			cfg.WebSearch.MaxResults = 12
		}
		if cfg.WebSearch.MaxContentChars == 0 {
			cfg.WebSearch.MaxContentChars = 18000
		}
		if cfg.WebSearch.MaxToolRounds == 0 {
			cfg.WebSearch.MaxToolRounds = 4
		}
		if time.Duration(cfg.WebSearch.Timeout) <= 0 {
			return Config{}, fmt.Errorf("web_search.timeout must be positive")
		}
		if cfg.WebSearch.MaxResults < 1 || cfg.WebSearch.MaxResults > 20 {
			return Config{}, fmt.Errorf("web_search.max_results must be between 1 and 20")
		}
		if cfg.WebSearch.MaxContentChars < 1000 || cfg.WebSearch.MaxContentChars > 50000 {
			return Config{}, fmt.Errorf("web_search.max_content_chars must be between 1000 and 50000")
		}
		if cfg.WebSearch.MaxToolRounds < 1 || cfg.WebSearch.MaxToolRounds > 20 {
			return Config{}, fmt.Errorf("web_search.max_tool_rounds must be between 1 and 20")
		}
	}

	for i := range cfg.Tasks {
		cfg.Tasks[i].ApplyDefaults(cfg.Timezone)
		if err := cfg.Tasks[i].Validate(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func applyDatabaseEnvironment(cfg *DatabaseConfig) {
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("CRONPILOT_DATABASE_DRIVER"))); value != "" {
		cfg.Driver = value
	}
	if value := strings.TrimSpace(os.Getenv("CRONPILOT_DATABASE_PATH")); value != "" {
		cfg.Path = value
	}
	if value := strings.TrimSpace(os.Getenv("MYSQL_ADDRESS")); value != "" {
		cfg.Address = value
		if strings.TrimSpace(cfg.Driver) == "" {
			cfg.Driver = "mysql"
		}
	}
	if value := strings.TrimSpace(os.Getenv("MYSQL_USERNAME")); value != "" {
		cfg.Username = value
	}
	if value := os.Getenv("MYSQL_PASSWORD"); value != "" {
		cfg.Password = value
	}
	if value := strings.TrimSpace(os.Getenv("MYSQL_DATABASE")); value != "" {
		cfg.Name = value
	}
	if strings.TrimSpace(cfg.Driver) == "" {
		cfg.Driver = "sqlite"
	}
	cfg.Driver = strings.ToLower(strings.TrimSpace(cfg.Driver))
	if cfg.Driver == "sqlite" && strings.TrimSpace(cfg.Path) == "" {
		cfg.Path = "cronpilot.db"
	}
	if cfg.Driver == "mysql" && strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "cronpilot"
	}
}

func validateDatabase(cfg *DatabaseConfig) error {
	switch cfg.Driver {
	case "sqlite":
		if strings.TrimSpace(cfg.Path) == "" {
			return fmt.Errorf("database.path is required for sqlite")
		}
	case "mysql":
		if strings.TrimSpace(cfg.Address) == "" {
			return fmt.Errorf("database.address or MYSQL_ADDRESS is required for mysql")
		}
		if strings.TrimSpace(cfg.Username) == "" {
			return fmt.Errorf("database.username or MYSQL_USERNAME is required for mysql")
		}
		if cfg.Password == "" {
			return fmt.Errorf("database.password or MYSQL_PASSWORD is required for mysql")
		}
		if strings.TrimSpace(cfg.Name) == "" {
			return fmt.Errorf("database.name or MYSQL_DATABASE is required for mysql")
		}
	default:
		return fmt.Errorf("database.driver must be sqlite or mysql")
	}
	return nil
}
