# 第三方系统集成指南

Catch 是一个**日志与错误接收端**，不是身份提供方。第三方系统集成分为两类：

1. **上报数据**：任何前端/后端服务通过项目 Ingest Key 把日志和错误发送给 Catch。
2. **登录 Catch**：使用租户配置的 OIDC 身份提供方登录控制台；这不会让 Catch 变成第三方系统的 IdP。

推荐为**每个外部系统/客户项目创建一个独立 Project**，使用独立 Ingest Key。Ingest Key 是只写密钥，只能上报，不能读取数据。

---

## 0. 准备

在 Catch 控制台：

1. 登录 → **设置 → 项目与密钥 → 新建项目**
2. 复制该项目的 Ingest Key（形如 `ck_xxxxxxxx`）
3. 记下 Catch 地址，例如：
   - 本地：`http://localhost:8091`
   - 生产：`https://catch.example.com`

可用环境变量：

```bash
CATCH_URL=https://catch.example.com
CATCH_KEY=ck_xxxxxxxx
```

验证连通性：

```bash
curl -s "$CATCH_URL/api/health"
# {"status":"ok","version":"1.0.0",...}
```

错误响应统一为 `{"error":{"code":"...","message":"..."}}`。每个响应带 `X-Request-Id`，排查时请保留。

---

## 1. 浏览器 SDK（推荐）

Catch 自带一个零依赖、可独立部署的浏览器 SDK：`/catch-sdk.js`。

### 1.1 引入

```html
<script
  src="https://catch.example.com/catch-sdk.js"
  data-key="ck_xxxxxxxx"
  data-env="production"
  data-release="partner-portal@1.4.0"
  data-service="partner-portal"
  crossorigin="anonymous"></script>
```

| 属性 | 必填 | 说明 |
| --- | --- | --- |
| `data-key` | 是 | 项目 Ingest Key；缺失时 SDK 自动禁用 |
| `data-env` | 否 | 环境，默认 `production` |
| `data-release` | 否 | 版本/发布号，便于定位回归 |
| `data-service` | 否 | 服务名，默认当前 hostname |
| `data-endpoint` | 否 | 自定义 ingest 地址；默认取 SDK 脚本所在源的 `/api/ingest` |
| `data-sample-rate` | 否 | 日志采样率 0–1，默认 1；错误不采样 |

Catch 对 `/catch-sdk.js` 返回 `Access-Control-Allow-Origin: *`，因此第三方站点无需额外配置静态资源 CORS。SDK 上报走 `/api/ingest`，该接口按项目 key 认证并允许任意 origin。

SDK 会自动：

- 捕获 `window.onerror`、`unhandledrejection`
- 包裹 `fetch`，记录 HTTP 面包屑并上报 5xx/网络错误
- 记录 SPA `pushState/replaceState/popstate` 页面浏览日志
- 记录最近 30 条面包屑（导航、点击、HTTP）
- 事件写入 localStorage 队列，批量上报；页面隐藏/卸载时用 `sendBeacon` 兜底
- 失败重试（最多 8 次），队列上限 100，不阻塞业务请求

### 1.2 主动 API

```js
// 关联业务用户（登录成功后调用）
Catch.setUser({ id: "u_123", email: "user@example.com", username: "alice" })

// 普通日志
Catch.log("info", "coupon applied", { couponId: "c_1", discount: 20 })

// 手动捕获错误
try {
  await submitOrder()
} catch (err) {
  Catch.captureError(err, { orderId: "ord_123", step: "submit" })
  throw err
}

// 添加面包屑
Catch.addBreadcrumb("business", "entered checkout", { cartItems: 3 })

// 强制发送
await Catch.flush()
```

级别支持：`debug | info | warn | error | fatal`。

### 1.3 React 错误边界

```tsx
class CatchErrorBoundary extends React.Component {
  componentDidCatch(error: Error, info: React.ErrorInfo) {
    window.Catch?.captureError(error, {
      mechanism: "react-error-boundary",
      componentStack: info.componentStack,
    })
  }
  render() {
    return this.state.hasError ? <Fallback /> : this.props.children
  }
}
```

可参考 Catch 自身的 `frontend/src/components/ErrorBoundary.tsx` 与 `frontend/src/lib/telemetry.ts`。

### 1.4 CSP

如果第三方站点启用了 Content-Security-Policy，需要放行：

```text
script-src  https://catch.example.com;
connect-src https://catch.example.com;
```

SDK 使用 `X-Ingest-Key` 请求头，因此 `connect-src` 必须包含 Catch 源。

### 1.5 版本锁定

生产建议固定版本或使用自己托管的副本：

```html
<script src="https://catch.example.com/catch-sdk.js?v=1.0.0" ...></script>
```

---

## 2. HTTP 采集 API（任何语言）

### 2.1 请求

```http
POST /api/ingest
X-Ingest-Key: ck_xxxxxxxx
Content-Type: application/json
```

也支持：

```http
Authorization: Bearer ck_xxxxxxxx
```

或 `POST /api/ingest?key=ck_xxxxxxxx`（适合 `sendBeacon`，日志中可能暴露 key，不推荐用于服务端）。

三种请求体：

```json
{ "kind": "log", "message": "single event" }
```

```json
[
  { "kind": "log", "message": "event 1" },
  { "kind": "error", "message": "event 2" }
]
```

```json
{ "events": [ { "kind": "log", "message": "event 1" } ] }
```

限制：

- 单次最多 **100 条**
- 请求体最大 **1 MiB**
- 默认每项目 **1200 条/分钟**，超限返回 `429` + `Retry-After`
- 字段超长会被截断；超大 context 会被替换为 `{"_truncated":true,"size":N}`

成功响应：

```json
{ "accepted": 1, "newIssues": 0, "reopenedIssues": 0 }
```

### 2.2 事件字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `kind` | `log` / `error` | 缺省根据 `type`/`error` 推断 |
| `source` | `frontend` / `backend` | 默认 `frontend` |
| `level` | `debug` / `info` / `warn` / `error` / `fatal` | 默认按 kind 推断 |
| `message` | string | 必填；错误可只给 `errorType` |
| `timestamp` | RFC3339 / Unix 秒 / 毫秒 | 缺省或异常时使用接收时间 |
| `errorType` | string | 如 `TypeError`；缺省从 message 推断 |
| `stack` | string | 堆栈文本 |
| `culprit` | string | 出错位置；缺省从 stack 提取首个应用帧 |
| `fingerprint` | string | 可选，8–128 位 `[A-Za-z0-9_-]`，自定义分组 |
| `url` | string | 页面/请求 URL |
| `environment` | string | `production` / `staging` / … |
| `release` | string | `web@1.2.3` |
| `service` | string | 服务名 |
| `platform` | string | `javascript-react` / `go` / … |
| `tags` | `map[string]string` | 最多 30 个，key ≤64，value ≤500 |
| `context` | object | 任意可 JSON 序列化上下文 |
| `user` | object | `{id,email,username}` |
| `request` | object | `{method,url,status,durationMs,requestId}` |
| `breadcrumbs` | array | `{timestamp,category,message,level,data}`，最多 50 |

### 2.3 错误示例

```bash
curl -X POST "$CATCH_URL/api/ingest" \
  -H "X-Ingest-Key: $CATCH_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "kind": "error",
        "source": "backend",
        "level": "error",
        "message": "MongoServerError: E11000 duplicate key error collection: shop.orders",
        "errorType": "MongoServerError",
        "stack": "MongoServerError: E11000\n    at orders.Repository.Insert (repository.go:78)",
        "environment": "production",
        "release": "orders-api@2.3.1",
        "service": "orders-api",
        "user": { "id": "u_123", "email": "user@example.com" },
        "request": {
          "method": "POST",
          "url": "/v1/orders",
          "status": 500,
          "durationMs": 183,
          "requestId": "req_abc"
        },
        "tags": { "region": "ap-east-1" },
        "context": { "orderId": "ord_123" }
      }
    ]
  }'
```

### 2.4 日志示例

```json
{
  "kind": "log",
  "source": "backend",
  "level": "warn",
  "message": "slow query: orders.find took 830ms",
  "service": "orders-api",
  "environment": "production",
  "context": { "collection": "orders", "durationMs": 830 },
  "tags": { "db": "mongo" }
}
```

### 2.5 源码跳转

在 Catch 中为项目配置源码仓库后，堆栈和 `culprit` 中的文件路径会生成源码链接。为了获得最佳效果：

- 上报原始堆栈，不要删除 `file:line:column` 信息
- JS 构建请保留 Source Map，并在发布前上传到项目的 **Settings → Source Maps**；服务端会把压缩帧还原为原始源码
- Source Map 的 `release` 必须与事件 `release`（SDK 的 `VITE_RELEASE`）完全一致
- CI 可以用 `POST /api/ingest/sourcemaps` + `X-Ingest-Key` 自动上传（见 `scripts/upload-sourcemaps.sh`）
- Source Map 内含 `sourcesContent` 时，用户可以在 Catch 内直接查看源码并高亮行号；否则回退到仓库链接
- 未上传 map 时，压缩 bundle 帧会保持纯文本；已入库的历史事件不会自动重写
- Go/Python/Java 等后端保留完整包路径或仓库内相对路径
- 在项目设置中填写正确的“源码根目录”（如 `frontend`、`backend`）

支持 GitHub、GitLab、Bitbucket、Gitea 和自定义 URL 模板。

### 2.6 分组规则

服务端按以下内容计算指纹：

```text
source + errorType + 归一化(message) + 归一化(culprit) → SHA-256 前 16 字节
```

归一化会替换 UUID、IP、邮箱、十六进制 ID、数字、引号内字符串。因此：

```text
User 123 not found
User 456 not found
```

会归并到同一个问题。若业务需要不同分组，传入自定义 `fingerprint`。

已解决的问题再次出现时自动重新打开，并增加回归次数。

---

### 2.7 OpenTelemetry(OTLP) 接入

Catch 原生接收 OTLP/HTTP logs 和 traces（protobuf、JSON、gzip），使用项目 Ingest Key 认证：

```text
POST /api/ingest/otlp/v1/logs
POST /api/ingest/otlp/v1/traces
# OTel 默认路径别名
POST /v1/logs
POST /v1/traces
X-Ingest-Key: ck_xxx
```

OTel Collector：

```yaml
exporters:
  otlphttp/catch:
    endpoint: https://catch.example.com/api/ingest/otlp
    headers:
      X-Ingest-Key: ck_xxx
    compression: gzip
```

SDK 环境变量：

```bash
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=https://catch.example.com/api/ingest/otlp
OTEL_EXPORTER_OTLP_HEADERS=X-Ingest-Key=ck_xxx
OTEL_SERVICE_NAME=orders-api
OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production,service.version=orders@2.3.1
```

说明：

- `service.name`、`service.version`、`deployment.environment` 会自动映射到服务/版本/环境
- severity ≥ ERROR 的日志和带 `exception.*` 的日志会转为 error 事件并进入问题分组
- traces 只保留 ERROR span 或带 `exception` 事件的 span，普通 span 不入库
- `exception.stacktrace` 可以继续走 Source Map 还原
- 请求体上限 20MB，单次最多 5000 条日志/错误 span
- metrics 暂不支持；gRPC 请通过 Collector 转 HTTP

## 3. 服务端集成示例

### 3.1 Go：简单 reporter

```go
package catch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Reporter struct {
	URL     string // https://catch.example.com/api/ingest
	Key     string // ck_xxx
	Service string
	Env     string
	Release string
	Client  *http.Client
}

func (r *Reporter) Report(ctx context.Context, kind, level, message string, fields map[string]any) error {
	event := map[string]any{
		"kind":        kind,
		"source":      "backend",
		"level":       level,
		"message":     message,
		"service":     r.Service,
		"environment": r.Env,
		"release":     r.Release,
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if fields != nil {
		event["context"] = fields
	}
	body, _ := json.Marshal(map[string]any{"events": []any{event}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ingest-Key", r.Key)
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("catch ingest: %s", resp.Status)
	}
	return nil
}
```

### 3.2 Go：slog handler + panic / 5xx

```go
func (r *Reporter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		started := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			if rec := recover(); rec != nil {
				_ = r.Report(req.Context(), "error", "fatal", fmt.Sprint(rec), map[string]any{
					"mechanism": "panic",
					"stack":     string(debug.Stack()),
					"path":      req.URL.Path,
				})
				http.Error(sw, "internal server error", 500)
				return
			}
			if sw.status >= 500 {
				_ = r.Report(req.Context(), "log", "error", "request failed", map[string]any{
					"method": req.Method, "path": req.URL.Path,
					"status": sw.status, "durationMs": time.Since(started).Milliseconds(),
					"requestId": req.Header.Get("X-Request-Id"),
				})
			}
		}()
		next.ServeHTTP(sw, req)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
```

> 更完整的 Go 实现可参考 Catch 自身的 `backend/internal/logging/logger.go`：异步队列 + slog fanout + 请求上下文。生产环境建议异步发送，避免阻塞业务。

### 3.3 Node.js / Express

```js
const CATCH_URL = process.env.CATCH_URL
const CATCH_KEY = process.env.CATCH_KEY

async function report(event) {
  try {
    await fetch(`${CATCH_URL}/api/ingest`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Ingest-Key": CATCH_KEY },
      body: JSON.stringify({ events: [{ source: "backend", ...event }] }),
    })
  } catch {
    // 采集失败不能影响业务
  }
}

app.use((req, res, next) => {
  const started = Date.now()
  res.on("finish", () => {
    if (res.statusCode >= 500) {
      report({
        kind: "log",
        level: "error",
        message: `${req.method} ${req.path} ${res.statusCode}`,
        environment: process.env.NODE_ENV,
        release: process.env.RELEASE,
        service: "partner-api",
        request: {
          method: req.method,
          url: req.originalUrl,
          status: res.statusCode,
          durationMs: Date.now() - started,
          requestId: req.headers["x-request-id"],
        },
      })
    }
  })
  next()
})

app.use((err, req, res, next) => {
  report({
    kind: "error",
    level: "error",
    message: err.message,
    errorType: err.name,
    stack: err.stack,
    environment: process.env.NODE_ENV,
    release: process.env.RELEASE,
    service: "partner-api",
    request: { method: req.method, url: req.originalUrl, status: 500 },
  })
  next(err)
})
```

### 3.4 Python / FastAPI

```python
import os, time, traceback
import httpx
from fastapi import FastAPI, Request

CATCH_URL = os.environ["CATCH_URL"]
CATCH_KEY = os.environ["CATCH_KEY"]
app = FastAPI()

async def report(event: dict) -> None:
    try:
        async with httpx.AsyncClient(timeout=5) as client:
            await client.post(
                f"{CATCH_URL}/api/ingest",
                headers={"X-Ingest-Key": CATCH_KEY},
                json={"events": [{"source": "backend", **event}]},
            )
    except Exception:
        pass

@app.middleware("http")
async def catch_middleware(request: Request, call_next):
    started = time.perf_counter()
    try:
        response = await call_next(request)
    except Exception as exc:
        await report({
            "kind": "error", "level": "error", "message": str(exc),
            "errorType": type(exc).__name__, "stack": traceback.format_exc(),
            "service": "partner-api", "environment": os.getenv("ENV", "production"),
            "request": {"method": request.method, "url": str(request.url), "status": 500},
        })
        raise
    if response.status_code >= 500:
        await report({
            "kind": "log", "level": "error",
            "message": f"{request.method} {request.url.path} {response.status_code}",
            "service": "partner-api", "environment": os.getenv("ENV", "production"),
            "request": {
                "method": request.method, "url": str(request.url),
                "status": response.status_code,
                "durationMs": int((time.perf_counter() - started) * 1000),
            },
        })
    return response
```

---

## 4. 关联与排障建议

- 生成并透传 `X-Request-Id`，同时写入 `request.requestId`，可在 Catch 中串联前后端。
- 登录后调用 `Catch.setUser({id,email})`，可以统计受错误影响的用户数。
- 每次发布设置新的 `release`，便于判断回归是否由新版本引入。
- 用 `environment` 区分生产/预发；用 `tags` 放 region、tenant、feature flag。
- 后端集成一定要异步、限时、失败静默，采集故障不能拖垮业务。
- 生产环境建议通过内网地址上报（例如 `http://catch.internal:8080`），浏览器 SDK 才使用公网地址。

---

## 5. 多租户 / 第三方客户接入

### 5.1 同一 Catch 租户下的多个外部系统

每个外部系统创建一个 Project：

```text
Acme 租户
├── Web App       ck_web_xxx
├── Orders API    ck_api_xxx
└── Partner Portal ck_partner_xxx
```

- 密钥之间不能互读，只能写入各自项目的事件。
- 控制台通过项目选择器过滤。
- 适合“本公司的多个服务”场景。

### 5.2 每个第三方客户一个 Catch 租户

如果外部客户需要**独立登录、独立成员、独立数据**：

```text
Catch 租户 A（acme）  → OIDC issuer A + 项目 A
Catch 租户 B（globex） → OIDC issuer B + 项目 B
```

- 每个租户可配置不同的 OIDC issuer/client，成员与事件完全隔离。
- 当前版本创建租户的方式：
  1. 启动时设置 `OIDC_TENANT_SLUG` / `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET`（会 upsert 该租户，可逐个租户执行）
  2. 或直接在 MongoDB 的 `tenants` 集合中预置（OIDC secret 需要通过应用加密后写入，通常不推荐手工操作）
- 面向第三方客户的自助开通/租户管理 API 尚未实现，属于后续路线图；当前建议由运维预置租户。

### 5.3 安全边界

- Ingest Key 只写；仍然建议每个项目/客户独立，不要跨客户复用。
- 管理 API 使用 HttpOnly 会话 Cookie，不要暴露给第三方。
- 如第三方服务端位于不可信网络，使用 HTTPS + 反向代理/白名单限制 ingest 入口。
- 定期在 Settings 轮换密钥；旧 key 立即失效。

---

## 6. OIDC 集成边界

- Catch 是 **OIDC 客户端**：它使用租户配置的 IdP（UCenter/Keycloak/Okta 等）登录控制台。
- Catch **不是** OIDC Provider，不能给第三方系统签发 token。
- 第三方系统如果也需要登录，应接入同一个 IdP，而不是接入 Catch。
- 回调和 issuer 必须从浏览器可访问。UCenter 对非 localhost 的 redirect URI 强制 HTTPS；通过公网 IP 测试 OIDC 时，需要 HTTPS 域名或 localhost/SSH 隧道。

---

## 7. 联调检查清单

```bash
# 1) 健康检查
curl -s "$CATCH_URL/api/health"

# 2) 无效 key 应返回 401
curl -i -X POST "$CATCH_URL/api/ingest" \
  -H "X-Ingest-Key: ck_invalid" \
  -H "Content-Type: application/json" \
  -d '{"kind":"log","message":"must fail"}'

# 3) 合法 key 写入
curl -s -X POST "$CATCH_URL/api/ingest" \
  -H "X-Ingest-Key: $CATCH_KEY" \
  -H "Content-Type: application/json" \
  -d '{"kind":"log","source":"backend","level":"info","message":"integration ok","service":"partner-api"}'

# 4) 打开控制台 → 日志 → 搜索 integration ok（选择正确项目和最近 1 小时）
```

常见问题：

| 现象 | 原因 / 处理 |
| --- | --- |
| `401 invalid_ingest_key` | key 错误、被轮换或项目已删除；到 Settings 复制最新 key |
| `429 rate_limited` | 超过 `INGEST_RATE_LIMIT`；降低批量/重试频率，或调大限制 |
| 日志页看不到事件 | 检查项目选择器、来源/级别/时间范围；注意 `source=backend` 的平台日志在“全部项目”下 |
| `请求来源不被允许` | 管理 API 的 Origin 被拒；把前端 origin 加入 `CORS_ORIGINS` 并重启（ingest 不受此限制） |
| 公网页面登录失败 | `CORS_ORIGINS` 需要包含 `http://<公网IP>:<端口>`；OIDC 还需要 HTTPS 回调 |
| 浏览器 SDK 无事件 | 检查 CSP `connect-src`、`data-key`、Network 中 `/api/ingest` 状态码 |
| OIDC `redirect_uri` 不匹配 | 在 IdP 中精确注册 `{PUBLIC_URL}/api/auth/oidc/callback` |
| 公网 IP 上 OIDC 失败 | UCenter 要求非 localhost 回调使用 HTTPS；为 Catch 配置 HTTPS 域名，或使用 localhost/SSH 隧道测试 |
| 事件被截断 | 单条消息、堆栈、context 有上限；大对象请拆分或只传关键字段 |

---

## 8. 参考

- 项目 README：OIDC、部署、API 一览
- `frontend/public/catch-sdk.js`：独立浏览器 SDK 源码
- `frontend/src/lib/telemetry.ts`：控制台自用的 React SDK
- `backend/internal/api/ingest_handlers.go`：采集协议实现
- `backend/internal/ingest/fingerprint.go`：错误归并规则
- `frontend/src/lib/source.ts`：堆栈路径解析与源码链接生成
