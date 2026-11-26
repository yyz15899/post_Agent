# Post Agent 前端

这是 post_Agent Go 项目的中文社区前端。包含按关键词或 ID 找帖、帖子详情、登录注册、发帖、赞同反对、净得分和 AI 搜帖。按 `feature/worker-concurrency` 的提交 `9fc3acc` 对接 API。

## Windows 本地联调

1. 启动项目的 MySQL、Redis、Milvus、User 服务和 Go 网关，按后端项目说明填写配置。AI 搜帖还需要后端的模型及向量服务配置。Go 网关默认端口为 8081。
2. 解压这个包。在解压目录打开 PowerShell，运行。

   ```powershell
   node server.mjs
   ```

3. 在浏览器访问 [http://127.0.0.1:3000](http://127.0.0.1:3000)。首页可直接搜索帖子。登录测试账号后再发帖、投票或使用 AI 搜帖。

只需要 Node.js，无需安装 npm 依赖。网关地址或前端端口不同时，可在启动前设置环境变量。

```powershell
$env:BACKEND_URL = "http://127.0.0.1:8081"
$env:FRONTEND_PORT = "3000"
node server.mjs
```

浏览器通过前端的 `/api/` 访问 Go 网关。Node 代理会保留请求方法、授权头和请求体，并直接转发 SSE 数据。双击 `public/index.html` 或访问 `http://127.0.0.1:3000/?mode=demo` 可以进入离线演示。演示中的帖子和回答都是示例，演示发帖和投票只保留在当前浏览器会话。

## 交互与接口

| 功能 | 路径 |
| --- | --- |
| 关键词搜索 | `GET /api/posts?keyword=...` |
| 帖子详情与分享链接 | `GET /api/posts/:id`，页面链接为 `/#post/:id` |
| 登录、注册 | `POST /api/signin` 和 `POST /api/signup` |
| 发帖 | `POST /api/posts` |
| 投票、分数、个人方向 | `POST /api/posts/:id/vote`、`GET /api/getsc/:pid`、`GET /api/getsc/:pid/user` |
| AI 搜帖 | `POST /api/agent/chat`，接收 SSE 的 `chunk`、`source` 和 `error` 事件 |

后端的搜索需要关键词，并且每次最多返回五篇。真实模式下，首页没有输入关键词时可以打开本次会话里发布的帖子，或输入帖子 ID。投票提交成功后，后台会异步计算净得分，页面会短时刷新数次，并保留手动刷新入口。登录凭据保存在浏览器的 sessionStorage，关闭对应标签会话后失效。

## Linux 部署

将 `public/` 放到 `/var/www/post-agent/public/`。把 `nginx.conf.example` 放到 Nginx 的站点配置目录，修改域名、静态目录和 Go 网关地址。这个方案由 Nginx 提供页面并代理 API，无需在服务器上运行 Node。

```bash
sudo nginx -t
sudo systemctl reload nginx
```

示例配置使用 HTTP。正式对外开放前，为你的域名配置 HTTPS，再用该域名登录测试。只通过反向代理开放站点，Go 网关、MySQL、Redis 等端口按实际部署限制在本机或内网。AI 搜帖应该逐段显示，部署后确认代理保留了 `proxy_buffering off`。Go 网关的 `/metrics` 未包含在该站点代理中，部署监控时由内网采集。
