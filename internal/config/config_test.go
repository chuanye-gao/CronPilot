package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAppliesTaskDefaults(t *testing.T) {
	path := writeConfig(t, `
timezone: Asia/Shanghai
server: {}
llm:
  model: test-model
  api_key: test-key
tasks:
  - name: morning-brief
    schedule: "0 8 * * *"
    prompt: Write a brief.
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Address != "127.0.0.1:8080" {
		t.Fatalf("server address = %q", cfg.Server.Address)
	}
	if cfg.Database.Path != "cronpilot.db" {
		t.Fatalf("database path = %q", cfg.Database.Path)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("database driver = %q", cfg.Database.Driver)
	}
	if cfg.Log.Format != "text" || cfg.Log.Level != "info" {
		t.Fatalf("log config = %#v", cfg.Log)
	}
	got := cfg.Tasks[0]
	if got.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q", got.Timezone)
	}
	if time.Duration(got.Timeout) != 5*time.Minute {
		t.Fatalf("timeout = %s", got.Timeout)
	}
	if got.Retry.MaxAttempts != 1 || time.Duration(got.Retry.Delay) != 10*time.Second {
		t.Fatalf("retry = %#v", got.Retry)
	}
}

func TestLoadMySQLFromWeixinCloudEnvironment(t *testing.T) {
	t.Setenv("MYSQL_ADDRESS", "10.0.0.8:3306")
	t.Setenv("MYSQL_USERNAME", "cronpilot")
	t.Setenv("MYSQL_PASSWORD", "mysql-secret")
	t.Setenv("MYSQL_DATABASE", "cronpilot_prod")
	path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.Driver != "mysql" || cfg.Database.Address != "10.0.0.8:3306" || cfg.Database.Username != "cronpilot" || cfg.Database.Name != "cronpilot_prod" {
		t.Fatalf("mysql config = %#v", cfg.Database)
	}
	if cfg.Database.Password != "mysql-secret" {
		t.Fatal("mysql password was not loaded")
	}
}

func TestLoadInfrastructureSettingsFromEnvironment(t *testing.T) {
	t.Setenv("CRONPILOT_SERVER_ADDRESS", "127.0.0.1:18080")
	t.Setenv("CRONPILOT_DATABASE_PATH", filepath.Join(t.TempDir(), "data", "cronpilot.db"))
	t.Setenv("CRONPILOT_LOG_FORMAT", "json")
	t.Setenv("CRONPILOT_LOG_LEVEL", "debug")
	path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.Path != os.Getenv("CRONPILOT_DATABASE_PATH") {
		t.Fatalf("database path = %q", cfg.Database.Path)
	}
	if cfg.Server.Address != "127.0.0.1:18080" {
		t.Fatalf("server address = %q", cfg.Server.Address)
	}
	if cfg.Log.Format != "json" || cfg.Log.Level != "debug" {
		t.Fatalf("log config = %#v", cfg.Log)
	}
}

func TestLoadWebSearchDefaultsAndEnvironment(t *testing.T) {
	t.Setenv("CRONPILOT_WEB_SEARCH_ENDPOINT", "http://search.internal:8080")
	path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.WebSearch.Enabled || cfg.WebSearch.Endpoint != "http://search.internal:8080" {
		t.Fatalf("web search = %#v", cfg.WebSearch)
	}
	if time.Duration(cfg.WebSearch.Timeout) != 15*time.Second || cfg.WebSearch.MaxResults != 12 || cfg.WebSearch.MaxContentChars != 18000 || cfg.WebSearch.MaxToolRounds != 4 {
		t.Fatalf("web search defaults = %#v", cfg.WebSearch)
	}
}

func TestLoadTavilyFromSingleEnvironmentVariable(t *testing.T) {
	t.Setenv("TAVILY_API_KEY", "tvly-test-secret")
	path := writeConfig(t, `
llm:
  model: test
  api_key: key
web_search:
  enabled: true
  provider: tavily
  endpoint: http://searxng:8080
tasks: []
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.WebSearch.Enabled || cfg.WebSearch.Provider != "tavily" {
		t.Fatalf("web search = %#v", cfg.WebSearch)
	}
	if cfg.WebSearch.Endpoint != "https://api.tavily.com" || cfg.WebSearch.APIKey != "tvly-test-secret" {
		t.Fatalf("Tavily config = %#v", cfg.WebSearch)
	}
}

func TestLoadGeminiFallbackFromEnvironment(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-test-secret")
	path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Gemini.APIKey != "gemini-test-secret" || cfg.Gemini.Model != "gemini-2.5-flash-lite" {
		t.Fatalf("Gemini config = %#v", cfg.Gemini)
	}
	if cfg.Gemini.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Fatalf("Gemini base URL = %q", cfg.Gemini.BaseURL)
	}
}

func TestLoadCloudflareRelayConfiguresGeminiAndTavily(t *testing.T) {
	t.Setenv("CRONPILOT_RELAY_URL", "https://relay.example.com/")
	t.Setenv("CRONPILOT_RELAY_KEY", "relay-secret")
	t.Setenv("GEMINI_API_KEY", "must-not-be-used")
	t.Setenv("TAVILY_API_KEY", "must-not-be-used")
	path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Relay.URL != "https://relay.example.com" || cfg.Relay.APIKey != "relay-secret" {
		t.Fatalf("relay config = %#v", cfg.Relay)
	}
	if cfg.Gemini.BaseURL != "https://relay.example.com/v1/gemini/openai" || cfg.Gemini.APIKey != "relay-secret" {
		t.Fatalf("Gemini relay config = %#v", cfg.Gemini)
	}
	if !cfg.WebSearch.Enabled || cfg.WebSearch.Provider != "tavily" || cfg.WebSearch.Endpoint != "https://relay.example.com/v1/tavily" || cfg.WebSearch.APIKey != "relay-secret" {
		t.Fatalf("Tavily relay config = %#v", cfg.WebSearch)
	}
}

func TestLoadRejectsIncompleteOrInsecureRelay(t *testing.T) {
	tests := []struct {
		name string
		url  string
		key  string
	}{
		{name: "missing key", url: "https://relay.example.com"},
		{name: "missing URL", key: "relay-secret"},
		{name: "remote HTTP", url: "http://relay.example.com", key: "relay-secret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CRONPILOT_RELAY_URL", test.url)
			t.Setenv("CRONPILOT_RELAY_KEY", test.key)
			path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")
			if _, err := Load(path); err == nil {
				t.Fatal("Load() succeeded, want relay validation error")
			}
		})
	}
}

func TestLoadAllowsLocalHTTPRelayForDevelopment(t *testing.T) {
	t.Setenv("CRONPILOT_RELAY_URL", "http://127.0.0.1:8787")
	t.Setenv("CRONPILOT_RELAY_KEY", "relay-secret")
	path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks: []\n")
	if _, err := Load(path); err != nil {
		t.Fatalf("Load() local relay error = %v", err)
	}
}

func TestLoadRejectsInvalidTaskSettings(t *testing.T) {
	tests := map[string]string{
		"schedule": `schedule: "not cron"`,
		"timezone": "schedule: \"0 8 * * *\"\n    timezone: Mars/Olympus",
		"timeout":  "schedule: \"0 8 * * *\"\n    timeout: -1s",
	}
	for name, taskSettings := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, "llm:\n  model: test\n  api_key: key\ntasks:\n  - name: invalid\n    "+taskSettings+"\n    prompt: test\n")
			if _, err := Load(path); err == nil {
				t.Fatal("Load() succeeded, want error")
			}
		})
	}
}

func TestLoadEmailCredentialsFromEnvironment(t *testing.T) {
	t.Setenv("TEST_SMTP_USER", "sender@example.com")
	t.Setenv("TEST_SMTP_PASSWORD", "smtp-secret")
	path := writeConfig(t, `
llm:
  model: test
  api_key: key
email:
  host: smtpdm.aliyun.com
  username_env: TEST_SMTP_USER
  password_env: TEST_SMTP_PASSWORD
tasks: []
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Email.Username != "sender@example.com" || cfg.Email.Password != "smtp-secret" || cfg.Email.From != "sender@example.com" {
		t.Fatalf("email config = %#v", cfg.Email)
	}
	if cfg.Email.Port != 465 || cfg.Email.TLS != "implicit" {
		t.Fatalf("email transport = %#v", cfg.Email)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cronpilot.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
