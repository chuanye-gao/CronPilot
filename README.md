# CronPilot

**Schedule AI tasks like cron jobs.**

CronPilot is a small, self-hostable scheduler for recurring AI tasks. Define a cron expression and a prompt, point CronPilot at any OpenAI-compatible model API, and let it execute the task on schedule.

The project deliberately starts with a narrow core:

```text
cron schedule -> AI task -> LLM -> result
```

Delivery channels, web search, MCP tools, persistence, retries, and a web UI can be layered on top without turning the scheduler itself into a large framework.

## Status

Early development. The current MVP supports:

- YAML task definitions
- Standard 5-field cron expressions
- Configurable timezone
- Multiple recurring tasks
- Enable/disable per task
- OpenAI-compatible `/chat/completions` endpoints
- API keys from environment variables
- Structured execution logs
- Graceful shutdown

## Quick start

Requirements: Go 1.24+ and an OpenAI-compatible API endpoint.

```bash
git clone https://github.com/chuanye-gao/CronPilot.git
cd CronPilot
cp cronpilot.example.yaml cronpilot.yaml
export CRONPILOT_API_KEY="your-api-key"
go run ./cmd/cronpilot -config cronpilot.yaml
```

On PowerShell:

```powershell
Copy-Item cronpilot.example.yaml cronpilot.yaml
$env:CRONPILOT_API_KEY="your-api-key"
go run ./cmd/cronpilot -config cronpilot.yaml
```

## Configuration

```yaml
timezone: Asia/Singapore

llm:
  base_url: https://api.openai.com/v1
  model: gpt-5
  api_key: ""

tasks:
  - name: daily-ai-brief
    schedule: "0 8 * * *"
    prompt: |
      Summarize the most important AI developments from the last 24 hours.
```

Keep `api_key` empty and use `CRONPILOT_API_KEY` for secrets whenever possible.

## Project structure

```text
cmd/cronpilot/        CLI entrypoint
internal/config/      YAML configuration
internal/task/        task model
internal/scheduler/   cron scheduling
internal/runner/      task execution
internal/llm/         model provider abstraction
```

## Roadmap

The intended evolution is incremental rather than framework-first:

1. Reliable cron + LLM execution core
2. Execution history and persistent task state
3. Retry, timeout, concurrency, and failure policies
4. Email and webhook delivery
5. Web search and tool calling
6. MCP integration
7. HTTP API and web UI
8. Multi-user/self-hosted deployment

## Design principle

CronPilot should make scheduled AI automation feel as simple as writing a cron job. The scheduler remains the center of the project; agent capabilities are extensions, not the foundation.
