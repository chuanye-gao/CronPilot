# 微信云托管上线清单

CronPilot 使用两个云托管服务：一个运行主应用，一个运行私有 SearXNG。主应用必须保持单实例常驻，避免定时任务漏跑或重复执行。

## 1. 数据库

在微信云 MySQL 中创建 `cronpilot` 数据库。CronPilot 启动时会自动创建和升级业务表。

主服务必须配置：

```text
MYSQL_ADDRESS=<微信云提供的内网地址:端口>
MYSQL_USERNAME=<数据库用户名>
MYSQL_PASSWORD=<数据库密码>
MYSQL_DATABASE=cronpilot
CRONPILOT_DATABASE_DRIVER=mysql
```

## 2. SearXNG 服务

从同一仓库创建第二个服务：

```text
Dockerfile: Dockerfile.search
构建目录: 仓库根目录
容器端口: 8080
最小实例: 1
最大实例: 1
```

不要把 SearXNG 暴露为一个无保护的公共搜索站点。优先使用同一环境的服务间访问地址，并把该地址配置到主服务的 `CRONPILOT_WEB_SEARCH_ENDPOINT`。

## 3. CronPilot 主服务

从现有 GitHub 仓库创建服务，不要复制官方 Go 计数器模板：

```text
仓库: chuanye-gao/CronPilot
分支: main
Dockerfile: Dockerfile
构建目录: 仓库根目录
容器端口: 8080
健康检查: /health/ready
最小实例: 1
最大实例: 1
```

在控制台的 Secret 或环境变量中配置：

```text
CRONPILOT_API_KEY=<DeepSeek API Key>
CRONPILOT_PUBLIC_URL=https://<正式域名>
CRONPILOT_SERVER_ADDRESS=0.0.0.0:8080
CRONPILOT_LOG_FORMAT=json
CRONPILOT_SMTP_USERNAME=<发件邮箱>
CRONPILOT_SMTP_PASSWORD=<SMTP 授权码>
CRONPILOT_WEB_SEARCH_ENDPOINT=<SearXNG 服务间地址>
```

不要在微信云配置本机的 `127.0.0.1:17891` 或 `host.docker.internal` 代理地址。云端如需代理，必须使用云端可以访问的代理服务。

## 4. 流水线

微信云流水线监听 `main` 分支 Push。GitHub CI 会先校验前端、Go、单元测试和生产镜像。生产发布建议通过 Pull Request 合并到 `main`，避免未通过 CI 的提交直接上线。

首次发布后依次验证：

1. `/health/live` 和 `/health/ready` 返回成功。
2. `/api/health` 显示 `storage: mysql` 且 WebSearch 健康。
3. 注册验证邮件可以收到，验证链接指向正式域名。
4. 新建一条测试任务并执行，输出包含真实来源链接。
5. 重启主服务后账号、任务和执行记录仍然存在。
6. 保持最小和最大实例都为 1，确认次日晨报只发送一次。
