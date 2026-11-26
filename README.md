# Post Agent

一个用 Go 编写的技术论坛。用户可以注册、登录、发帖、搜帖和投票，也可以用站内 AI 根据帖子内容回答问题，打开回答引用的原帖。

网关用 Gin 提供 HTTP 接口，通过 gRPC 调用用户服务。投票状态和同步任务在同一个 MySQL 事务中写入，由后台 Worker 同步至 Redis。新帖由后台索引任务写入 Milvus，Eino Agent 通过语义检索查找相关帖子，前端用 SSE 逐段显示回答。

- [发帖与向量同步流程](micro_user/docs/images/post-sync-flow.svg)
- [Agent 工具调用流程](micro_user/docs/images/agent-flow.svg)
- [前端接口与 Nginx 部署说明](post_agent_frontend/README.md)

## 目录结构

```text
micro_lhh/
├── micro_user/           Go 后端、数据库脚本和 Docker Compose 配置
├── post_agent_frontend/  前端页面、本地代理和 Nginx 示例
└── README.md
```

将前端解压到与 `micro_user/` 同级的位置。解压后应能从仓库根目录找到 `post_agent_frontend/server.mjs`。

## Windows 本地启动

### 1. 准备环境

安装 Docker Desktop（使用 Linux 容器和 WSL 2）、Go（满足 [go.mod](micro_user/go.mod) 中的版本要求）和 Node.js（建议 22 或更新版本）。前端没有 npm 依赖。准备火山方舟的向量模型配置和 DeepSeek 的 API 配置，用于发帖向量化与 AI 搜帖。

### 2. 配置 `micro_user/.env`

以下值是示例。用自己的本地密码、密钥和模型配置替换占位符。`MYSQL_DSN` 的密码要与现有 MySQL 用户的密码一致。已有 Docker 数据卷时，修改 `MYSQL_ROOT_PASSWORD` 不会更改库里的密码。

```dotenv
MYSQL_ROOT_PASSWORD=replace_with_local_password
MYSQL_DSN=root:replace_with_local_password@tcp(127.0.0.1:3307)/my_db?charset=utf8mb4&parseTime=True&loc=Local
redis.Addr=127.0.0.1:16380
redis.pwd=
JWT_SECRET=replace_with_a_random_secret
ARK_API_KEY=replace_with_ark_api_key
EMBEDDER=replace_with_ark_embedding_model_or_endpoint
DEEPSEEK_API_KEY=replace_with_deepseek_api_key
MODEL_LLM=replace_with_deepseek_model_name
```

项目默认 Redis 容器未设置密码，`redis.pwd` 留空。向量模型的输出维度需要与代码中的 `post_docs` 集合配置一致，目前是 **2048**。保留 `.env` 在本机，提交时只提交脱敏示例。

### 3. 启动 MySQL、Redis 和 Milvus

从仓库根目录进入后端目录再执行 Compose，方便它读取同目录的 `.env`。

```powershell
cd .\micro_user
docker compose up -d mysql redis
docker compose ps
docker compose exec -T redis redis-cli ping
```

MySQL 和 Redis 容器应为 `Up`，Redis 应返回 `PONG`。Windows 侧访问端口分别为 `3307` 和 `16380`。

**首次使用空数据库时**，在同一个终端导入基础表。

```powershell
docker compose cp .\sql\init.sql mysql:/tmp/post-agent-init.sql
docker compose exec mysql mysql -uroot -p my_db
```

在 MySQL 提示符输入下面的命令。初始化后应能看到 `users` 和 `posts`。投票和同步任务相关的表由网关启动时创建或迁移。如果已有 `users` 和 `posts`，可以跳过导入。

```sql
SOURCE /tmp/post-agent-init.sql
SHOW TABLES;
EXIT;
```

在已有容器中启动 Milvus，或根据 [Milvus 官方 Windows 安装指南](https://milvus.io/docs/install_standalone-windows.md) 单独启动 Milvus Standalone。当前项目 Compose 文件管理 MySQL 和 Redis，Go 网关连接 `localhost:19530` 上的 Milvus，使用 `default` 数据库。

```powershell
Test-NetConnection localhost -Port 19530
```

### 4. 启动 Go 服务和前端

依次打开三个 PowerShell 窗口。每个窗口从仓库根目录执行对应的命令，并保持窗口运行。

| 窗口 | 目录 | 命令 | Windows 端口 |
| --- | --- | --- | --- |
| 用户服务 | `cd .\micro_user` | `go run ./usersvr` | `8973` |
| Go 网关 | `cd .\micro_user` | `go run ./gateway` | `8081` |
| 前端 | `cd .\post_agent_frontend` | `node server.mjs` | `3000` |

网关启动时会连接 Milvus 并初始化向量模型。前端将同源的 `/api/` 请求代理到 `http://127.0.0.1:8081`。

```powershell
curl.exe -fsS -o NUL -w "HTTP %{http_code}\n" http://127.0.0.1:8081/metrics
```

打开 [http://127.0.0.1:3000](http://127.0.0.1:3000)，确认顶部显示 **后端模式**。

### 5. 验证完整链路

1. 注册或登录测试账号，发布一篇标题带「联调测试」的帖子。
2. 搜索「联调测试」，打开帖子详情，确认标题和正文正确。
3. 点赞，等后台任务同步后确认净得分加一。可以手动刷新分数。
4. 等帖子同步到 Milvus 后，在 AI 搜帖页面按标题和正文中的关键词提问。确认回答能逐段显示，并且引用指向对应帖子。

只想看界面时，可打开 [离线演示](http://127.0.0.1:3000/?mode=demo)。演示使用示例数据。

## 常见启动问题

| 现象 | 排查方法 |
| --- | --- |
| MySQL 报 `Access denied` | 核对实际库账号、密码、`MYSQL_DSN` 及端口 `3307`。 |
| 用户服务报表不存在 | 检查是否在 `my_db` 中导入了 `sql/init.sql`。 |
| 网关启动时报 Redis 或 Milvus 连接失败 | 检查 Redis 的 `16380` 和 Milvus 的 `19530`，以及 `ARK_API_KEY`、`EMBEDDER` 的配置。 |
| 前端出现 `502` | 先确认 Go 网关的 `/metrics` 有响应，再检查前端是否代理到正确的地址。 |
| 发帖成功，AI 暂时找不到 | 检查网关的向量同步日志，确认帖子已写入 `post_docs`。 |
| `/metrics` 返回 200，但日志报 `traces export … TLS handshake` | 核对 Trace 接收端的协议和端口。配置入口是 [tracing.go](micro_user/gateway/tracing.go)。 |

如果本机 Trace 接收端确实在 `4317` 上提供明文 OTLP/gRPC，可在 `micro_user/.env` 设置下面一行，然后重启网关。若在当前 PowerShell 设过同名环境变量，也要同步更新。使用 TLS 的接收端应保留其 HTTPS 配置。

```dotenv
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://127.0.0.1:4317
```



