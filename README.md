# CronPilot

**Schedule AI tasks like cron jobs.**

CronPilot is a small, self-hostable scheduler for recurring AI tasks. Define a cron expression and a prompt, point CronPilot at any OpenAI-compatible model API, and let it execute the task on schedule.

CronPilot combines a focused scheduling engine with a built-in web console:

```text
cron schedule -> AI task -> execution -> LLM -> observable result
```

Delivery channels, local web research, persistence, retries, and a web UI are layered on top without turning the scheduler itself into a large framework.

## Status

Early development. The current MVP supports:

- YAML task definitions
- Standard 5-field cron expressions
- Configurable timezone
- Multiple recurring tasks
- Enable/disable per task
- OpenAI-compatible `/chat/completions` endpoints
- API keys from environment variables
- Per-task timeout and fixed-delay retry
- Manual task runs
- SQLite task and execution persistence
- REST API for tasks and executions
- React/TypeScript product site, account screens, and management console
- Conversational AI task builder with structured drafts and real test runs
- Email-verified accounts, Argon2id passwords, and persistent sessions
- Per-user task and execution isolation
- Structured execution logs and graceful shutdown
- Model-driven `web_search` and `web_open` tool calls
- Tavily live search and managed article extraction, enabled by one environment variable
- Optional self-hosted SearXNG search backend
- Safe public-page extraction with private-network blocking, size limits, and prompt-injection boundaries
- Source links and publication metadata for current-information tasks

## Quick start

Requirements: Go 1.24+ and an OpenAI-compatible API endpoint.

```bash
git clone https://github.com/chuanye-gao/CronPilot.git
cd CronPilot
cp cronpilot.example.yaml cronpilot.yaml
export CRONPILOT_API_KEY="your-api-key"
go run ./cmd/cronpilot -config cronpilot.yaml
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080) after CronPilot starts. The production frontend is embedded in the Go binary, so deployment does not need a separate frontend runtime.

Tasks and execution history are stored in `cronpilot.db` by default. Production deployments can use MySQL by setting the environment variables documented below. YAML tasks are imported only when the database is empty; after that, the database is the source of truth.

On PowerShell:

```powershell
Copy-Item cronpilot.example.yaml cronpilot.yaml
$env:CRONPILOT_API_KEY="your-api-key"
go run ./cmd/cronpilot -config cronpilot.yaml
```

## Configuration

```yaml
timezone: Asia/Singapore

server:
  address: 127.0.0.1:8080

database:
  path: cronpilot.db

log:
  format: text
  level: info

llm:
  base_url: https://api.openai.com/v1
  model: gpt-5
  api_key: ""

tasks:
  - name: daily-ai-brief
    schedule: "0 8 * * *"
    timeout: 5m
    retry:
      max_attempts: 3
      delay: 10s
    prompt: |
      Summarize the most important AI developments from the last 24 hours.
```

Keep `api_key` empty and use `CRONPILOT_API_KEY` for secrets whenever possible.

### Gemini fallback

Add `GEMINI_API_KEY` to enable Gemini as the automatic fallback for both task runs and the task creation assistant:

```text
GEMINI_API_KEY=your-gemini-api-key
```

DeepSeek remains the primary model. CronPilot calls Gemini only when the primary request fails, returns an empty response, omits required research evidence, or begins with a clear refusal. Canceled and timed-out tasks are never retried through the fallback. The default is the low-cost `gemini-2.5-flash-lite`; override it with `GEMINI_MODEL` if needed. CronPilot uses Google's [OpenAI-compatible Gemini endpoint](https://ai.google.dev/gemini-api/docs/openai), so the same protected web-research tool loop works with both providers.

### MySQL

CronPilot automatically switches to MySQL when `MYSQL_ADDRESS` is present, or when `CRONPILOT_DATABASE_DRIVER=mysql` is set. It creates the target database when permitted and applies its tables automatically on startup:

```text
CRONPILOT_DATABASE_DRIVER=mysql
MYSQL_ADDRESS=10.0.0.8:3306
MYSQL_USERNAME=cronpilot
MYSQL_PASSWORD=your-database-password
MYSQL_DATABASE=cronpilot
```

Keep the database password in the deployment platform's secret manager. SQLite remains the local default and requires no MySQL variables.

## Web research with Tavily

CronPilot owns the complete model tool loop. When a task needs current or externally verifiable information, the model can call `web_search` repeatedly, open selected sources with `web_open`, identify gaps, and continue researching before producing the final answer.

For hosted deployments, add the following secret. It is never exposed to the browser or returned by the health API:

```text
TAVILY_API_KEY=tvly-your-key
```

The presence of `TAVILY_API_KEY` automatically enables Tavily Search and Extract. The explicit equivalent is:

```yaml
web_search:
  enabled: true
  provider: tavily
  endpoint: https://api.tavily.com
  api_key_env: TAVILY_API_KEY
  timeout: 15s
  max_results: 12
  max_content_chars: 18000
  max_tool_rounds: 10
```

Tavily searches use the news topic and recency window requested by the model. Opening a result uses Tavily Extract, which is more reliable than downloading arbitrary publisher pages from a mainland-hosted container. Search and extraction errors are retained in execution logs without logging the API key.

SearXNG remains available as an optional self-hosted provider by setting `provider: searxng` and its internal `endpoint`.

Web pages are treated as untrusted evidence. `web_open` only accepts public HTTP/HTTPS destinations, rejects loopback and private network addresses, limits redirects and response sizes, removes scripts/navigation/forms, and clearly tells the model to ignore instructions embedded in page content. Important current claims should still be confirmed by independent sources.

## Email notifications

CronPilot can send a test message from the web console and notify recipients after a task succeeds, fails, or times out. Aliyun DirectMail uses an SMTP account and SMTP password—not an Alibaba Cloud AccessKey. Keep both values in environment variables:

```powershell
$env:CRONPILOT_SMTP_USERNAME="sender@example.com"
$env:CRONPILOT_SMTP_PASSWORD="your-smtp-password"
```

Then enable the SMTP transport in `cronpilot.yaml`:

```yaml
email:
  host: smtpdm.aliyun.com
  port: 465
  username_env: CRONPILOT_SMTP_USERNAME
  password_env: CRONPILOT_SMTP_PASSWORD
  from: sender@example.com
  tls: implicit
  timeout: 20s
```

Open the **Email** page in the web console, enter a recipient, and send a test message. Test recipients are not stored. Task-specific notification settings can then be added in YAML or through the task editor:

```yaml
delivery:
  type: email
  to:
    - you@example.com
  on: [success, failed, timeout]
  include_output: true
```

Delivery failures are recorded separately from task execution status. A successfully completed AI task remains successful even if its notification email cannot be sent.

Task changes made in the web console take effect immediately and persist across restarts. Executions left pending or running by an unexpected shutdown are marked `interrupted` the next time CronPilot starts.

## Accounts

Registration requires a working email transport. CronPilot sends a 30-minute verification link, stores passwords using Argon2id, and keeps only hashed verification and session tokens in SQLite. Login sessions use an HttpOnly, SameSite cookie and expire after 30 days.

Tasks and execution history belong to the authenticated user. Existing tasks from an earlier single-user installation are assigned to the first registered account.

## Docker

Create a local `.env` file, then start the deployment:

```powershell
@"
CRONPILOT_API_KEY=your-deepseek-api-key
TAVILY_API_KEY=your-tavily-api-key
GEMINI_API_KEY=your-gemini-api-key
CRONPILOT_SMTP_USERNAME=your-qq-address@qq.com
CRONPILOT_SMTP_PASSWORD=your-qq-smtp-authorization-code
"@ | Set-Content .env
docker compose up --build -d
```

Open [http://127.0.0.1:18080](http://127.0.0.1:18080). The included Docker configuration uses DeepSeek's OpenAI-compatible API with `deepseek-v4-flash` and QQ Mail SMTP for local deployment. Change `CRONPILOT_PUBLIC_URL` when the service is exposed through a LAN address or HTTPS domain so email verification links point to the reachable address.

The Compose deployment stores SQLite data in the `cronpilot-data` volume, runs CronPilot as a non-root user with a read-only root filesystem, and checks `/health/ready`. SQLite deployments must run exactly one CronPilot replica to avoid duplicate scheduled executions.

Application logs are written to stdout. Set `CRONPILOT_LOG_FORMAT=json` for structured production logs and use the container platform to collect and retain them.

## Weixin Cloud Run

Use the existing GitHub repository instead of copying Weixin Cloud's Go counter template. The production Dockerfile builds the React frontend and Go backend from a clean checkout. Configure the main service with port `8080`, readiness path `/health/ready`, and exactly one always-on instance. The in-process scheduler does not yet support multiple active replicas.

Add `TAVILY_API_KEY` to the main service's secrets. A second search service is no longer required. The older private SearXNG deployment remains documented as an optional fallback for installations that explicitly choose it.

See [the Weixin Cloud launch checklist](deploy/weixin-cloud.md) for required secrets, MySQL variables, service settings, and first-release verification.

## Frontend development

The frontend source lives in `web/`. It uses React, TypeScript, Vite, and hash-based routes so the same build works from the embedded Go server.

```powershell
cd web
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run build
```

The production build is written to `internal/api/static/` and embedded into the next Go build. During UI development, `pnpm run dev` starts Vite and proxies API and health requests to the Go server on port 8080.

## HTTP API

```text
POST   /api/auth/register
POST   /api/auth/verify
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/auth/me
GET    /api/health
GET    /health/live
GET    /health/ready
GET    /api/tasks
POST   /api/tasks
GET    /api/tasks/{id}
PUT    /api/tasks/{id}
DELETE /api/tasks/{id}
POST   /api/tasks/{id}/run
POST   /api/task-assistant/plan
POST   /api/task-assistant/test
GET    /api/tasks/{id}/executions
GET    /api/executions
GET    /api/executions/{id}
GET    /api/email/status
POST   /api/email/test
```

All task, execution, and email-management endpoints require an authenticated session. Health and account-entry endpoints remain public.

The API and web console use the same task store, scheduler, and runner. Scheduled and manual runs therefore share timeout, retry, status, and execution-history behavior.

## Project structure

```text
cmd/cronpilot/        CLI entrypoint
internal/config/      YAML configuration
internal/api/         REST API and embedded web console
internal/auth/        accounts, password hashing, verification, sessions
internal/delivery/    SMTP transport and email templates
internal/execution/   execution model and statuses
internal/task/        task model
internal/scheduler/   cron scheduling
internal/runner/      task execution
internal/llm/         model provider abstraction
internal/storage/     storage abstraction and SQLite implementation
internal/websearch/   local search agent, page extraction, and web safety
web/                  React/TypeScript frontend source
```

## Roadmap

The intended evolution is incremental rather than framework-first:

1. Reliable cron + LLM execution core
2. Execution history and persistent task state
3. Retry, timeout, concurrency, and failure policies
4. Email and webhook delivery
5. Web search and tool calling
6. HTTP API and web UI
7. Multi-user/self-hosted deployment
8. MCP integration and additional local tools

## Design principle

CronPilot should make scheduled AI automation feel as simple as writing a cron job. The scheduler remains the center of the project; agent capabilities are extensions, not the foundation.
