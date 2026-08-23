# go-template

一个面向中小型服务的 Go 单体模板，业务代码按领域模块内聚，默认接入 Fiber v3、GORM、PostgreSQL、Redis 和 SCS 服务端会话

## 目录

- [架构](#架构)
- [技术栈](#技术栈)
- [目录结构](#目录结构)
- [手动依赖注入](#手动依赖注入)
- [路由](#路由)
- [双登录域认证](#双登录域认证)
- [权限校验](#权限校验)
- [参数校验与统一响应](#参数校验与统一响应)
- [环境配置](#环境配置)
- [数据库迁移](#数据库迁移)
- [快速启动](#快速启动)
- [数据访问](#数据访问)
- [质量检查](#质量检查)
- [自动部署](#自动部署)
- [发布前初始化](#发布前初始化)

## 架构

业务按领域放在 `internal/modules/<domain>`，每个模块内部包含 API、Service 和 Repository：

```text
Fiber
  ↓
App / Admin Auth Realm
  ↓
Domain API
  ↓
Domain Service
  ↓
Domain Repository
  ↓
Models / Data
  ├── GORM 数据库模型
  ├── GORM / PostgreSQL
  └── go-redis / Redis
```

- `api.go` 负责路由 参数绑定和 HTTP 响应转换
- `service.go` 使用具体 `Service` 结构体承载业务规则 流程和事务边界
- `repository.go` 使用具体 `Repository` 结构体保存手写查询和数据库模型转换
- `models` 只保存 GORM 数据库模型 不包含查询和业务逻辑
- `data` 统一管理 PostgreSQL、Redis、事务和连接生命周期
- `auth` 提供 App、Admin 双登录域及权限校验门面
- `health` 直接提供系统探针，不套用业务分层
- `app` 是唯一组合根并通过构造函数手动注入依赖

Service 和 Repository 构造函数直接返回具体指针。Service 可以声明消费者所有的私有最小接口用于注入 Fake，但持久层固定只有一个 GORM 实现，不创建公开 Repository 接口或 `repository_gorm.go`

项目不使用全局 `handler/logic/types/dal/transport` 横向目录，也不使用 ServiceContext

根目录 `main.go` 保持为唯一进程入口

## 技术栈

- Go 1.25 或更高版本
- Fiber v3
- GORM v2 和 PostgreSQL 驱动
- golang-migrate v4 和 PGX v5 迁移驱动
- go-redis
- SCS v2 和 goredisstore
- go-playground/validator
- YAML 配置
- Docker Compose
- GitHub Actions

项目包含 `toolchain` 指令，启用默认的 `GOTOOLCHAIN=auto` 时会自动下载兼容工具链

## 目录结构

```text
.
├── main.go                         # 进程入口与系统信号处理
├── configs/
│   ├── config.local.yaml           # 本地环境配置
│   ├── config.stg.yaml             # 预发布环境配置
│   └── config.production.yaml      # 生产环境配置
├── internal/
│   ├── app/                        # 手动依赖注入和应用生命周期
│   ├── apperror/                   # 业务码和应用错误链
│   ├── auth/                       # 双登录域 服务端会话和权限门面
│   ├── modules/                    # 按领域内聚的业务模块
│   │   ├── user/                   # 业务用户
│   │   ├── adminuser/              # 管理员用户
│   │   ├── config/                 # 系统配置
│   │   ├── health/                 # 存活 就绪和数据库探针
│   │   └── identity/               # 账号模块共享的最小认证契约
│   ├── models/                     # GORM 数据库模型
│   ├── config/                     # YAML 配置加载与校验
│   ├── data/                       # PostgreSQL Redis 和事务
│   ├── migration/                  # golang-migrate 执行能力
│   ├── httpserver/                 # Fiber 与网络生命周期
│   └── httpx/                      # 参数校验和统一 HTTP 响应
├── migrations/                     # PostgreSQL 版本化迁移
├── tools/                           # 项目开发命令和初始化器
├── docs/                           # 架构说明
├── deploy/                         # 远端部署配置和脚本
├── tests/                          # 集中存放测试文件
└── AGENTS.md                       # 项目协作规范
```

新增订单业务时创建：

```text
internal/modules/order/
├── api.go
├── service.go
└── repository.go
```

业务变复杂后可以在同一模块增加 `entity.go` `errors.go` 或按业务动作拆分文件，不增加无业务含义的中间目录

## 手动依赖注入

应用层按照 Repository、Service、API 的顺序显式组装业务模块：

```go
userService := user.NewService(
	user.NewRepository(resources),
	passwords,
	auth.NewSessionIssuer(realms.App),
)
user.RegisterHandlers(server.App(), userService, realms.App)
```

系统探针不创建 Service 和 Repository：

```go
health.RegisterHandlers(
	server.App(),
	resources.Ping,
	resources.PingDatabase,
	cfg.HTTP.ReadinessTimeout,
)
```

这种写法与参考仓库一致，依赖关系可以直接通过构造函数阅读，不引入运行时 DI 容器

## 路由

Fiber 路由直接写在所属业务包的 `api.go`：

```go
// RegisterHandlers 注册用户模块的 HTTP 路由
func RegisterHandlers(router fiber.Router, service *Service) {
	resource := resource{service: service}
	users := router.Group("/api/v1/users")

	users.Post("/", resource.create)
	users.Get("/:id", resource.get)
}
```

当前模板不使用顶层 `api/routes.yaml`，也不生成路由、API、Service 或 Repository

只有未来采用完整 OpenAPI、Proto 或 Thrift 契约时，才需要重新增加顶层 `api` 或 `idl` 目录

## 双登录域认证

模板使用 SCS v2 管理 Redis 服务端会话，不依赖 Sa-Token-Go，也不把权限状态固化到自包含 JWT 中：

- `realms.App` 服务 `/api/**`，`realms.Admin` 服务 `/admin/**`
- 两个 Realm 可以对应同一用户表，但使用独立 Redis 键空间，令牌不能跨端复用
- 客户端只通过 `Authorization: Bearer <token>` 传递令牌
- Redis 会话键和反向索引只保存 SHA-256 令牌摘要，不保存原始令牌
- 同时支持当前会话注销、指定设备下线和用户全部下线
- 默认会话绝对有效期为 30 天，空闲有效期为 7 天，活动请求会刷新空闲时间

登录接口属于实际账号业务。密码、短信验证码或第三方凭据校验成功后，由注入账号 Service 的对应 Realm 创建会话：

```go
token, err := s.sessions.Issue(ctx, user.ID, device)
if err != nil {
	return identity.Token{}, err
}
return token, nil
```

用户和管理员 Service 会先校验数据库中的真实密码哈希和账号状态，再签发对应登录域会话。`identity.Token` 返回 `access_token`、`token_type`、`expires_at` 和 `realm`，客户端后续发送：

```http
Authorization: Bearer <access_token>
```

受保护路由绑定对应 Realm，中间件会把可信主体写入请求上下文：

```go
users := router.Group("/api/users", appRealm.AuthenticateMiddleware())
users.Get("/me", resource.me)

principal, err := auth.RequirePrincipal(c.Context())
if err != nil {
	return err
}
result, err := r.service.GetProfile(c.Context(), principal.UserID)
```

模板已经提供以下账号接口：

| 登录域 | 方法与路径 | 用途 |
| --- | --- | --- |
| App | `POST /api/auth/register` | 注册业务用户 |
| App | `POST /api/auth/login` | 业务用户登录 |
| App | `GET /api/users/me` | 查询当前业务用户 |
| App | `POST /api/auth/logout` | 注销当前 App 会话 |
| Admin | `POST /admin/auth/login` | 管理员登录 |
| Admin | `GET /admin/users/me` | 查询当前管理员 |
| Admin | `POST /admin/users` | 创建管理员用户 |
| Admin | `POST /admin/auth/logout` | 注销当前 Admin 会话 |

账号禁用、密码重置或需要立即收回权限时调用 `realms.LogoutUser(ctx, userID)`，仅下线某个设备时调用对应 Realm 的 `LogoutDevice`

## 权限校验

简单业务默认使用数据库角色和权限表。业务 Repository 实现最小的 `auth.PermissionChecker` 接口，再在组合根创建管理端权限门面：

```go
authorizer, err := auth.NewAuthorizer(realms.Admin, permissionRepository)
if err != nil {
	return err
}

admins.Get(
	"/admin/users",
	realms.Admin.AuthenticateMiddleware(),
	authorizer.RequirePermission("system:user:read"),
	resource.list,
)
```

这样可以保持调用方式稳定，同时保留手写 SQL 和业务内聚。只有出现复杂角色继承、多租户域或策略关系时，才需要让 `PermissionChecker` 的实现接入 Casbin

## 参数校验与统一响应

Fiber 在请求绑定阶段自动执行 go-playground/validator 规则，API 只需要声明标签并返回绑定错误：

```go
type createRequest struct {
	Name  string `json:"name" validate:"required,min=2,max=32"`
	Email string `json:"email" validate:"required,email"`
}

func (r resource) create(c fiber.Ctx) error {
	var request createRequest
	// 请求绑定会自动执行结构体校验
	if err := c.Bind().Body(&request); err != nil {
		return err
	}

	result, err := r.service.Create(c.Context(), request.Name, request.Email)
	if err != nil {
		return err
	}
	return httpx.Created(c, result)
}
```

JSON 请求采用严格解析，未知字段、非法类型、多个连续 JSON 值都会返回参数格式错误

成功响应固定使用 `code` `message` `data`：

```json
{
  "code": 0,
  "message": "成功",
  "data": {
    "id": 1
  }
}
```

参数校验失败会返回字段名、规则、规则参数和中文提示：

```json
{
  "code": 40001,
  "message": "请求参数校验失败",
  "data": {
    "fields": [
      {
        "field": "email",
        "rule": "email",
        "message": "email必须是有效的邮箱地址"
      }
    ]
  }
}
```

业务码 `0` 表示成功，失败业务码采用 `<HTTP 状态码><两位序号>`：

| 业务码 | HTTP 状态 | 含义 |
| --- | --- | --- |
| `40000` | 400 | 参数格式错误 |
| `40001` | 400 | 参数校验失败 |
| `40100` | 401 | 未登录或登录失效 |
| `40101` | 401 | 认证会话缺失或失效 |
| `40102` | 401 | 敏感操作需要近期认证 |
| `40300` | 403 | 无权访问 |
| `40301` | 403 | 当前主体缺少指定权限 |
| `40400` | 404 | 资源不存在 |
| `40900` | 409 | 资源状态冲突 |
| `50000` | 500 | 服务器内部错误 |
| `50300` | 503 | 服务暂不可用 |
| `50310` | 503 | 会话存储暂不可用 |
| `50311` | 503 | 权限数据暂不可用 |

业务包可以在相同 HTTP 前缀下定义细分码，例如 `40901` 表示用户名已存在：

```go
return apperror.Wrap(40901, "用户名已存在", err)
```

底层错误会保留在错误链中供服务端日志使用，不会写入客户端响应

访问日志会同时记录 HTTP 状态码和 `business_code`，便于按业务失败原因检索请求

## 环境配置

仓库提供三个运行环境，不使用 `.env` 文件：

| 环境 | 配置文件 | 用途 |
| --- | --- | --- |
| `local` | `configs/config.local.yaml` | 本地开发和联调 |
| `stg` | `configs/config.stg.yaml` | 预发布服务器 |
| `production` | `configs/config.production.yaml` | 生产服务器 |

PostgreSQL 和 Redis 的连接信息、密码和连接池参数直接保存在对应 YAML 中

认证配置包含绝对有效期、空闲有效期以及 App、Admin Redis 键前缀。三个环境使用不同前缀，避免共享 Redis 时发生会话串用：

```yaml
auth:
  session_lifetime: 720h
  idle_timeout: 168h
  app:
    key_prefix: "go-template:local:auth:app:"
  admin:
    key_prefix: "go-template:local:auth:admin:"
```

配置采用严格解析，未知字段或非法环境名会导致应用启动失败

## 数据库迁移

项目使用 `golang-migrate` 管理版本化 SQL。GORM 负责模型映射、查询和业务事务，不调用 `AutoMigrate`。迁移器不会创建 PostgreSQL 服务、逻辑数据库、用户或权限，目标数据库必须先存在。

创建下一组迁移文件：

```bash
go tool migrate create -ext sql -dir migrations -seq add_order_status
```

官方 CLI 根据现有最大版本生成成对文件，不会覆盖已有内容：

```text
migrations/000002_add_order_status.up.sql
migrations/000002_add_order_status.down.sql
```

`golang-migrate` 官方 CLI 通过 `go.mod` 的 `tool` 指令固定版本，创建迁移时不需要单独安装系统级命令。数据库执行入口使用同版本的官方库和 PGX v5 驱动。

编译后的服务和 `go run .` 使用同一组迁移子命令：

```bash
go run . migrate up -config configs/config.local.yaml -path migrations
go run . migrate version -config configs/config.local.yaml -path migrations
go run . migrate down -steps 1 -config configs/config.local.yaml -path migrations
```

以上命令要求配置中的 PostgreSQL 地址能从当前机器访问。默认本地配置使用 Compose 服务名，因此直接使用仓库配置时应在 Compose 容器中运行：

```bash
docker compose run --rm migrate
docker compose run --rm migrate migrate version -config /app/configs/config.local.yaml -path /app/migrations
docker compose run --rm migrate migrate down -steps 1 -config /app/configs/config.local.yaml -path /app/migrations
```

迁移文件使用六位递增版本和 snake_case 名称。已合并或已经在共享环境执行的文件不可修改、重命名或删除，只能新增修复版本。生产变更必须兼容上一个应用版本，应用回滚不会自动回滚数据库。完整规则见 [持久层架构](docs/persistence.md)。

## 快速启动

使用 Compose 启动 PostgreSQL 和 Redis，执行一次 `migrate up`，成功后再启动应用：

```powershell
docker compose up --build
```

检查服务：

```powershell
Invoke-RestMethod http://localhost:3000/healthz
Invoke-RestMethod http://localhost:3000/readyz
Invoke-RestMethod http://localhost:3000/api/v1/ping
```

- `/healthz` 只检查 HTTP 进程是否存活
- `/readyz` 同时检查 PostgreSQL 和 Redis
- `/api/v1/ping` 执行 PostgreSQL 轻量查询并返回数据库连接状态

直接运行 Go 进程时，需要先执行迁移，并确保配置中的 PostgreSQL 和 Redis 地址可以从当前网络访问：

```powershell
go run . migrate up -config configs/config.local.yaml -path migrations
go run . -config configs/config.local.yaml
```

## 数据访问

Service 通过所属文件内的私有最小接口调用 Repository：

```go
user, err := s.repository.FindByID(ctx, id)
```

具体 Repository 直接使用共享 Data 和 GORM：

```go
result := r.data.DB(ctx).First(&user, "id = ?", id)
value, err := r.data.Redis().Get(ctx, key).Result()
```

GORM 数据库模型统一写在 `internal/models`，CRUD 和自定义查询写在所属模块的 `repository.go`，不生成持久层代码，也不拆分接口和 GORM 实现

跨多个数据操作时由 Service 确定事务边界，Repository 继续传递事务上下文，Redis 缓存只在数据库事务成功后更新或失效

完整约束见 [持久层架构](docs/persistence.md)

## 质量检查

```powershell
go test ./...
go vet ./...
go build ./...
```

执行真实 PostgreSQL 和 Redis 集成测试：

```powershell
docker compose up -d postgresql redis
docker compose exec postgresql dropdb --if-exists -U go_template go_template_test
docker compose exec postgresql createdb -U go_template go_template_test
$env:TEST_POSTGRES_URL = "postgres://go_template:go_template_local@127.0.0.1:5432/go_template_test?sslmode=disable"
$env:TEST_REDIS_ADDRESS = "127.0.0.1:6379"
$env:TEST_REDIS_PASSWORD = "go_template_local"
go test ./...
```

所有 `*_test.go` 文件统一放在顶层 `tests` 目录

## 自动部署

部署流程参考 [Linghe Java Backend Template](https://github.com/aiShuiJiaoDeXioShou/linghe-java-backend-template)：

| Git 分支 | GitHub Environment | 配置文件 | 部署目标 |
| --- | --- | --- | --- |
| `main` | `stg` | `config.stg.yaml` | 预发布环境 |
| `rel` | `production` | `config.production.yaml` | 生产环境 |

`.github/workflows/deploy.yml` 会先执行测试和静态检查，再把服务、配置和迁移文件打包并通过 SSH 发布。远端在旧版本继续服务时构建新镜像，用新镜像执行 `migrate up`，迁移成功后才启动新版应用。

迁移失败时发布立即停止，旧应用继续运行。新版健康检查失败时只恢复上一个应用版本，数据库不会自动执行 `down`，因此删除列或收紧约束需要采用分阶段的 expand migrate contract 变更。

GitHub 的 `stg` 和 `production` Environments 需要分别设置以下 Variables：

| Variable | 说明 |
| --- | --- |
| `SSH_HOST` | 部署服务器地址 |
| `SSH_USER` | SSH 用户 |
| `SSH_PORT` | SSH 端口 |
| `SSH_HOST_FINGERPRINT` | SSH ED25519 主机指纹 |
| `DEPLOY_DIR` | `/opt` 下的独立部署目录 |
| `APP_PORT` | 对外暴露端口 |
| `GOARCH` | 可选目标架构，默认 `amd64` |

默认使用 `SSH_PASSWORD` Environment Secret，数据库和 Redis 凭据直接读取仓库内 YAML

远端服务器需要准备 Docker、Docker Compose、`1panel-network` 网络，以及网络别名为 `postgresql` 和 `redis` 的外部服务

## 发布前初始化

当前模板名称和模块名保留为 `go-template`，初始化真实项目时需要替换仓库内所有 `go-template`，并同步修改数据库名称、账号、密码、镜像名称和部署包名称
