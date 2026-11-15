# Catch

Catch 是一个多租户日志与错误可观测平台：前端 SDK 和后端服务把日志/错误统一上报，平台负责归并问题、展示堆栈与上下文、追踪回归，并把数据转化为可执行的改进建议。

- **前端**：React 19 + TypeScript + Vite + TailwindCSS v4 + shadcn/ui + Recharts
- **后端**：Go + chi + MongoDB
- **认证**：多租户 OIDC（Authorization Code + PKCE + nonce + state），已使用 UCenter 完成端到端验证
- **部署**：前端构建产物由 Go 直接托管，可单容器运行（`/app/web` + 单个二进制）

---

## 功能

| 模块 | 能力 |
| --- | --- |
| 概览 | 错误/日志趋势、级别分布、受错误影响的用户数、高频问题、最近错误、30 秒自动刷新 |
| 问题 | 指纹归并、状态流转（待处理/已解决/已忽略）、回归检测、负责人、评论、堆栈/面包屑/上下文 |
| 日志 | 前端 + 后端统一检索，来源/级别/时间/关键词过滤，10 秒实时刷新，详情弹窗 |
| 持续改进 | 健康分、MTTR、回归数、长期未复现问题、错误环比、激增检测、规则化改进建议 |
| 设置 | 项目与采集密钥、密钥轮换、成员角色、租户信息、OIDC 配置、SDK 接入示例与一键测试 |
| 可观测性 | 后端请求日志/panic 自动落库；前端 SDK 捕获 `window.onerror`、`unhandledrejection`、React 错误边界和 API 5xx |

---

## 架构

```
┌─────────────────────────────┐         ┌──────────────────────────────┐
│  React SPA (Vite build)     │         │  OIDC Provider (UCenter)     │
│  - shadcn/ui + Tailwind     │         │  /t/acme                     │
│  - Catch Browser SDK        │         │  每个租户独立 issuer          │
│  - 错误边界 / 页面浏览日志    │         └──────────────┬───────────────┘
└──────────────┬──────────────┘                        │
               │ /api/*（同源，由 Go 提供）               │ 授权码 + PKCE
               ▼                                        ▼
┌──────────────────────────────────────────────────────────────────────┐
│  Go API (chi)                                                        │
│  - 会话 Cookie（HttpOnly, SameSite=Lax, 服务端 Session）              │
│  - OIDC 登录 / 租户隔离 / RBAC（owner/admin/member/viewer）           │
│  - /api/ingest 采集（项目 Ingest Key + 速率限制）                      │
│  - 问题指纹归并 / 回归检测 / 统计 / 改进洞察                            │
│  - slog 双写：stdout + MongoDB（source=backend）                      │
│  - 静态托管 frontend/dist（SPA fallback）                             │
└──────────────────────────────┬───────────────────────────────────────┘
                               ▼
                    ┌─────────────────────┐
                    │  MongoDB            │
                    │  tenants/users/...  │
                    │  events/issues/...  │
                    │  TTL 自动过期        │
                    └─────────────────────┘
```

---

## 快速开始

### 依赖

- Go 1.27+
- Node.js 20+ / pnpm 12
- MongoDB 7/8（本地 `127.0.0.1:27017` 即可）

```bash
cp .env.example .env              # 按需修改
cp frontend/.env.example frontend/.env
make install
```

### 方式 A：开发模式（API :8091 + Vite :5173）

```bash
# 终端 1
make dev-api

# 终端 2
make dev-web
```

打开 <http://localhost:5173>，选择演示租户 `acme`，使用开发登录 `demo@acme.dev`（种子数据已预填）。

开发模式默认配置：

```
PUBLIC_URL=http://localhost:8091
APP_URL=http://localhost:5173
CORS_ORIGINS=http://localhost:5173,http://localhost:8091
```

Vite 会把 `/api` 代理到 `http://localhost:8091`。

### 方式 B：Go 单进程托管前端（推荐部署方式）

```bash
make build     # pnpm build + go build
make run       # 或直接 ./backend/bin/catch
```

打开 <http://localhost:8080>。Go 会：

- 从 `STATIC_DIR`（未设置时自动探测 `web`、`../frontend/dist`、`frontend/dist`、`/app/web`）读取前端
- 对 `/assets/*` 输出长期缓存头，其余 SPA 路由回退到 `index.html`
- 同源提供 `/api/*`，无需额外 Nginx

单进程托管时务必让 **PUBLIC_URL = APP_URL = 对外访问地址**，OIDC 回调为：

```
{PUBLIC_URL}/api/auth/oidc/callback
```

### 方式 C：Docker Compose

```bash
docker compose up --build
```

打开 <http://localhost:8080>。容器内只运行 Go 进程（前端已复制到 `/app/web`）和 MongoDB。

> 演示种子会创建租户 `acme`、项目 `Web App` / `Orders API`、开发账号 `demo@acme.dev`，并生成 7 天演示数据。
> 演示采集密钥固定为 `ck_demo_web_0000000000000000000000` 与 `ck_demo_api_0000000000000000000000`。
> **生产环境请关闭 `SEED_DEMO` 和 `ALLOW_DEV_LOGIN`，修改 `APP_SECRET`，并在 Settings 中轮换密钥。**

---

## OIDC 多租户（含 UCenter 实测）

### 模型

- OIDC 配置按租户存储在 MongoDB 中（issuer、clientId、加密后的 clientSecret、scopes、允许邮箱域名）。
- 登录流程：`/api/auth/oidc/start?tenant=acme` → 身份提供方 → `/api/auth/oidc/callback`。
- 使用 `state` + `nonce` + PKCE（S256）；state 一次性消费并存在于 TTL 集合。
- 回调后根据 `(tenantId, subject)` 创建租户内用户；首次登录可按邮箱域名限制。
- **租户内第一个用户自动成为 owner**；之后用户使用租户 `defaultRole`（默认 member）。
- 每个用户只属于当前租户，会话中固定 `tenantId`，所有查询强制带租户过滤。
- 角色：`owner > admin > member > viewer`。viewer 只读；member 可处理问题/评论；admin 可管理项目、成员和 OIDC。

### 在 UI 中配置

登录后进入 **设置 → 租户与 SSO**：

1. 打开“启用 OIDC 登录”
2. 填写 Issuer / Client ID / Client Secret
3. 可选：额外 Scopes（如 `roles`）、允许邮箱域名（如 `acme.test`）
4. 复制页面显示的回调地址，在身份提供方中注册
5. 保存后点击“测试 OIDC 登录”

也可以通过环境变量在启动时自动配置一个租户：

```bash
OIDC_TENANT_SLUG=acme
OIDC_ISSUER=http://localhost:8188/t/acme
OIDC_CLIENT_ID=app_demo_acme
OIDC_CLIENT_SECRET=demo-secret-acme
OIDC_ALLOWED_DOMAINS=acme.test
```

### UCenter 端到端验证（已执行）

本机 UCenter 运行于 `http://localhost:8188`，演示租户 `acme`：

| 项目 | 值 |
| --- | --- |
| Issuer | `http://localhost:8188/t/acme` |
| Discovery | `http://localhost:8188/t/acme/.well-known/openid-configuration` |
| Client ID | `app_demo_acme` |
| Client Secret | `demo-secret-acme` |
| 测试用户 | `owner@acme.test` / `Password123!` |
| 回调地址 | `http://localhost:8091/api/auth/oidc/callback`（Go 托管 :8080 时对应改成 :8080） |
| 允许域名 | `acme.test` |

实测结果：

- `/api/auth/oidc/start` 生成带 PKCE + nonce 的授权 URL
- UCenter 登录后回调 `/api/auth/oidc/callback`，返回 `302` 到 `APP_URL + returnTo`
- ID Token 验签、nonce 校验通过，创建租户内用户并写入 Session Cookie
- `/api/auth/me` 返回 `owner@acme.test`、`role=member`（演示租户已有成员；新租户第一个用户会是 owner）
- 受保护的 `/api/issues` 返回 200，数据严格限定在该租户
- 前端登录页点击“使用 OIDC 单点登录”也完成了同样的端到端流程

> UCenter 每个租户是独立 issuer（如 `.../t/globex`）。Catch 的每个租户也可以各自指向不同 issuer，实现真正的多租户 SSO。

**第二个租户验证（globex）**：启动时追加一组 `OIDC_TENANT_SLUG=globex` / `OIDC_ISSUER=http://localhost:8188/t/globex` / 对应 client 配置重启，即可 upsert 第二个租户。实测使用 UCenter 的 `owner@globex.test / Password123!` 登录后：

- 回调成功，`/api/auth/me` 返回 `tenant=globex`，用户为 `owner`（该租户第一个用户）
- 项目数 = 0、问题数 = 0，看不到任何 acme 的数据
- 日志只包含 globex 自己的请求日志

这验证了 OIDC 多租户的租户级 issuer、独立用户映射和数据隔离。

### Keycloak 示例

```bash
OIDC_TENANT_SLUG=acme
OIDC_ISSUER=https://keycloak.example.com/realms/acme
OIDC_CLIENT_ID=catch
OIDC_CLIENT_SECRET=<secret>
OIDC_ALLOWED_DOMAINS=acme.com
```

Keycloak Client 需开启：

- Client authentication：On（confidential）
- Valid redirect URIs：`https://catch.example.com/api/auth/oidc/callback`
- Standard flow：On
- PKCE：S256（推荐）

---

## 采集 API

### 认证

项目 Ingest Key 通过以下任一方式传递：

```
X-Ingest-Key: ck_xxx
Authorization: Bearer ck_xxx
?key=ck_xxx
```

Ingest Key 是**只写密钥**，可安全放入浏览器构建产物；它只能写入事件，不能读取数据。可在 **设置 → 项目与密钥** 轮换。

### 请求

`POST /api/ingest`，支持：

- 单条事件对象
- 事件数组 `[...]`
- 批量对象 `{"events":[...]}`

限制：单次最多 100 条、1MB；默认每项目 1200 条/分钟（`INGEST_RATE_LIMIT`）。

```json
{
  "events": [
    {
      "kind": "error",
      "source": "frontend",
      "level": "error",
      "message": "TypeError: x is undefined",
      "stack": "TypeError: x is undefined\n    at App (App.tsx:42:7)",
      "errorType": "TypeError",
      "url": "https://app.example.com/checkout",
      "environment": "production",
      "release": "web@1.2.3",
      "service": "web",
      "platform": "javascript-react",
      "user": { "id": "u_123", "email": "user@example.com" },
      "tags": { "region": "ap-east-1" },
      "breadcrumbs": [
        { "timestamp": "2026-01-02T03:04:05Z", "category": "http", "message": "GET /api/orders 200" }
      ]
    },
    {
      "kind": "log",
      "source": "backend",
      "level": "warn",
      "message": "slow query: orders.find took 830ms",
      "service": "orders-api",
      "context": { "durationMs": 830, "collection": "orders" },
      "request": { "method": "GET", "url": "/api/orders", "status": 200, "requestId": "req_123" }
    }
  ]
}
```

响应：

```json
{ "accepted": 2, "newIssues": 1, "reopenedIssues": 0 }
```

### 错误归并

- 指纹 = `source + errorType + 归一化 message + culprit` 的 SHA-256 前 16 字节。
- 归一化会替换 UUID、十六进制 ID、IP、邮箱、数字、引号内字符串，避免 `user 123` / `user 456` 分裂。
- 客户端可通过 `fingerprint` 自定义分组（8–128 位字母数字/下划线/连字符）。
- 已解决问题再次出现时自动重新打开并累加 `regressionCount`。

### 前端 SDK

`frontend/src/lib/telemetry.ts` 是内置 SDK：

```bash
VITE_INGEST_KEY=ck_demo_web_0000000000000000000000
VITE_API_URL=          # 与 API 同域时留空
VITE_ENV=production
VITE_RELEASE=web@1.2.3
```

- 自动捕获 `window.onerror`、`unhandledrejection`、React ErrorBoundary、API 5xx/网络错误
- 维护最近 30 条 breadcrumb，随错误一起上报
- 事件先写 localStorage 队列，批量发送；页面隐藏/卸载时使用 `sendBeacon` 兜底
- 失败最多重试 8 次，队列上限 200，绝不阻塞业务请求
- 设置页“测试日志 / 测试错误”可验证采集链路

### 后端日志自动捕获

Go 后端使用 slog 双写：

- stdout：JSON 结构化日志
- MongoDB：`source=backend`、`kind=log` 的事件（含 requestId、method、path、status、durationMs、tenant）

请求日志、4xx、5xx、panic 堆栈都会出现在“日志”页的“后端”来源中；日志页也支持按项目过滤（平台级后端日志在“全部项目”下展示）。事件按 `RETENTION_DAYS` 自动 TTL 过期。

---

## 持续改进

`/api/insights` 根据当前租户/项目和 7 或 30 天窗口计算：

- **健康分**：从未处理 fatal/error、回归、错误环比、MTTR、日志错误率、长期未复现问题等维度扣分（0–100，A–E）
- **MTTR**：已解决事件的 `resolvedAt - firstSeen` 平均值
- **回归**：解决后再次出现的问题数量
- **激增检测**：最近 24h vs 前 24h 按问题的错误量变化
- **Top 噪音占比**：Top 1 问题占 Top 5 错误量的比例
- **规则化建议**：
  - 未处理的 fatal 错误 → 立即处理
  - 回归 → 补充回归测试、复盘修复
  - 错误量环比上升 ≥20% → 检查发布/依赖/流量
  - 日志错误率 ≥5% → 定位高频后端错误
  - MTTR > 72h → 建立 SLA 与负责人
  - 超过 7 天未复现 → 关闭或忽略降低噪音
  - 新增 > 解决 → 安排缺陷治理时间
  - Top 1 占比 ≥30% → 优先治理单点噪音

前端“持续改进”页展示健康分、指标、趋势、建议和“捕获 → 归并 → 定级 → 修复 → 验证 → 复盘”闭环。

---

## API 一览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/health` | 健康检查（含 Mongo ping） |
| GET | `/api/auth/tenants` | 公开的租户登录目录（不含密钥） |
| GET | `/api/auth/oidc/start` | 生成 OIDC 授权 URL（PKCE/nonce/state） |
| GET | `/api/auth/oidc/callback` | OIDC 回调，建立会话 |
| POST | `/api/auth/dev` | 开发登录（仅 `ALLOW_DEV_LOGIN=true`） |
| POST | `/api/auth/logout` | 注销并删除服务端会话 |
| GET | `/api/auth/me` | 当前用户、租户、权限 |
| POST | `/api/ingest` | 采集日志/错误（Ingest Key） |
| GET | `/api/overview` | 概览统计 + 时间序列 |
| GET | `/api/insights` | 健康分、改进指标与建议 |
| GET | `/api/issues` | 问题列表（状态/级别/来源/搜索/排序/分页） |
| GET | `/api/issues/{id}` | 问题详情 + 最近事件 + 评论 + 趋势 |
| PATCH | `/api/issues/{id}` | 变更状态 / 指派负责人 |
| POST | `/api/issues/{id}/comments` | 添加评论 |
| GET | `/api/events` | 日志/错误检索（时间、来源、级别、关键词、分页） |
| GET | `/api/events/{id}` | 事件详情 |
| GET | `/api/projects` | 项目列表（非 Admin 密钥脱敏） |
| POST | `/api/projects` | 创建项目（Admin+） |
| PATCH | `/api/projects/{id}` | 更新项目（Admin+） |
| POST | `/api/projects/{id}/rotate-key` | 轮换密钥（Admin+） |
| DELETE | `/api/projects/{id}` | 删除项目（Admin+） |
| GET | `/api/members` | 成员列表 |
| PATCH | `/api/members/{id}` | 修改角色（Admin+） |
| DELETE | `/api/members/{id}` | 移除成员（Admin+） |
| GET | `/api/tenant` | 租户信息（OIDC secret 不返回） |
| PATCH | `/api/tenant` | 修改租户名称（Admin+） |
| PUT | `/api/tenant/oidc` | 修改 OIDC 配置（Admin+） |

错误响应统一为：

```json
{ "error": { "code": "invalid_body", "message": "…", "details": {} } }
```

每个响应包含 `X-Request-Id`，后端 5xx 会记录到日志页，并在前端错误上下文中带上该 ID。

---

## 数据与安全

- **多租户隔离**：所有查询强制 `tenantId`；项目、成员、问题、日志、评论、会话均按租户隔离。
- **OIDC client secret**：AES-256-GCM 加密后存 MongoDB，密钥由 `APP_SECRET` 派生，永不返回浏览器。
- **会话**：32 字节随机 token，仅存 SHA-256；HttpOnly、SameSite=Lax，生产建议 `COOKIE_SECURE=true`。
- **CSRF**：Cookie 会话 + Origin 校验 + SameSite；写操作要求 JSON。
- **速率限制**：按项目对 ingest 做固定窗口限流，超限返回 429 + `Retry-After`。
- **输入限制**：1MB 请求体、100 条/批、字段截断、上下文超限标记 `_truncated`。
- **保留策略**：Mongo TTL 索引按 `RETENTION_DAYS` 清理事件；问题与成员保留。
- **开发登录**：生产环境（`APP_ENV=production`）强制关闭；`APP_SECRET` 为默认值时拒绝启动。
- **演示数据**：仅在 `SEED_DEMO=true` 时写入固定演示密钥，生产必须关闭。

---

## 测试

```bash
make test
# 等价于
cd backend && go test ./...
cd frontend && pnpm typecheck && pnpm build
```

已覆盖：

- 指纹归一化/稳定性、错误类型/culprit 提取
- OIDC 邮箱域名白名单、returnTo 开放重定向防护
- ingest 单条/数组/批量解析、时间戳兼容、标签清洗
- UCenter OIDC 端到端：授权 → 登录 → 回调 → ID Token 校验 → 会话 → 受保护 API
- Playwright 页面冒烟：登录、概览、问题详情、日志、持续改进、设置、OIDC 登录
- 前端 SDK 实际写入：页面浏览日志、React 渲染错误、设置页测试事件

---

## 目录结构

```
catch/
├── backend/
│   ├── cmd/server/           # 入口：配置、Mongo、种子、优雅退出
│   └── internal/
│       ├── api/              # 路由与 handlers（auth/ingest/issues/events/insights/...）
│       ├── auth/             # 多租户 OIDC + PKCE + nonce
│       ├── config/           # 环境变量
│       ├── httpx/            # JSON、中间件、CORS、Origin 校验
│       ├── ingest/           # 指纹归一化、级别映射
│       ├── logging/          # slog stdout + MongoDB 双写（异步队列）
│       ├── model/            # 领域模型
│       ├── reqctx/           # 请求上下文身份/trace
│       ├── secure/           # AES-GCM、随机 token、SHA-256
│       ├── seed/             # 演示租户/项目/7 天数据
│       └── store/            # MongoDB 仓储、索引、聚合统计
├── frontend/
│   ├── src/components/ui/    # shadcn/ui 组件
│   ├── src/components/       # 布局、图表、通用组件
│   ├── src/pages/            # 登录/概览/问题/日志/改进/设置
│   ├── src/lib/              # api、telemetry、types、utils
│   └── src/context/          # 认证、项目选择
├── Dockerfile                # Node 构建前端 + Go 编译 + 单镜像运行
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## 已知边界与后续改进

- OIDC `roles` claim 目前未映射为 Catch 角色；可增加“租户级 claim → 角色”映射策略。
- 尚未实现告警通道（Webhook/邮件/钉钉）与错误预算。
- 尚未接入 Source Map 还原压缩后的前端堆栈。
- 问题合并、相似问题推荐、自动静音规则可作为下一步。
- 日志检索目前使用正则 + 分页；数据量增大后建议引入全文索引或专用日志存储。
