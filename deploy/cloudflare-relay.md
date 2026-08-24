# Cloudflare Worker 海外能力中继

CronPilot 的主应用仍部署在微信云。Cloudflare Worker 只中继 Tavily Search、Tavily Extract 和 Gemini Chat Completions 三个固定接口，不代理任意网址，也不负责登录、任务调度、数据库或邮件。

## 1. 创建独立的中继密钥

生成一段至少 32 字节的随机字符串，作为 `CRONPILOT_RELAY_KEY`。它不是 Tavily 或 Gemini Key，只用于微信云主服务向 Worker 证明身份。

## 2. 发布 Worker

### 方式 A：直接连接当前 GitHub 仓库（推荐）

在 Cloudflare 的 Workers & Pages 中选择 **Import a repository**，连接 `chuanye-gao/CronPilot`，并设置：

```text
生产分支: main
Root directory: relay
Build command: 留空
Deploy command: pnpm run deploy
Build watch include path: relay/*
```

Cloudflare 中的 Worker 名称必须是 `cronpilot-relay`，与 `relay/wrangler.jsonc` 的 `name` 一致。以后只有 `relay/` 内的变更才需要触发 Worker 发布；主应用代码的普通变更无需重复发布 Worker。

然后在 Worker 的 **Settings / Variables & Secrets** 中添加三个运行时 Secret：

```text
CRONPILOT_RELAY_KEY
TAVILY_API_KEY
GEMINI_API_KEY
```

不要把它们误放到只在构建期间可用的 Build variables 中。

### 方式 B：本机 Wrangler 手动发布

进入仓库的 `relay` 目录：

```bash
npm install
npx wrangler login
npx wrangler secret put CRONPILOT_RELAY_KEY
npx wrangler secret put TAVILY_API_KEY
npx wrangler secret put GEMINI_API_KEY
npm run deploy
```

三个值都通过 Cloudflare Secret 保存，不要写入 `wrangler.jsonc`、Git 或前端代码。发布完成后先访问：

```text
https://<worker地址>/health
```

预期返回 `{"status":"ok","service":"cronpilot-relay"}`。

## 3. 绑定域名（推荐）

可以先使用 `workers.dev` 地址测试。正式环境建议在 Cloudflare Worker 的 Settings / Domains & Routes 中添加自定义域名，例如：

```text
relay.example.com
```

域名必须由当前 Cloudflare 账号管理。Worker 端没有 Cookie 和浏览器跨域需求，因此无需开放 CORS。

## 4. 配置微信云主服务

微信云只需要新增：

```text
CRONPILOT_RELAY_URL=https://relay.example.com
CRONPILOT_RELAY_KEY=<与 Worker 中相同的独立随机密钥>
```

启用中继后，可以从微信云主服务移除 `TAVILY_API_KEY` 和 `GEMINI_API_KEY`。DeepSeek、MySQL 和 SMTP 配置不变。CronPilot 会自动把 Gemini 和 Tavily 请求改发到 Worker。

## 5. 验证

重新发布微信云服务并登录 CronPilot，打开“系统状态”页面：

1. 页面顶部应显示正在通过 Cloudflare Relay 连接。
2. 分别点击 Gemini 和 Tavily 的“真实测试”。
3. Tavily 测试会消耗一次搜索额度。
4. Worker 的 `/health` 只代表 Worker 在线；管理页面中的真实测试才会同时验证密钥和上游服务。

## 安全边界

- Worker 只接受三个写死的上游路径，不能用作通用代理。
- 除 `/health` 外，所有接口都要求 Bearer 鉴权。
- 请求体上限为 1 MB，上游请求有超时限制。
- Worker 会覆盖客户端提交的 Tavily Key；真实服务 Key 不会返回给主应用。
- 如果中继密钥泄露，只需同时轮换 Cloudflare Secret 和微信云环境变量。
