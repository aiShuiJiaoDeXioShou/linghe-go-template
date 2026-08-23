# 持久层架构

## 默认结构

持久化代码跟随所属业务模块，项目固定使用 GORM，不拆分 Repository 接口和 GORM 实现：

```text
API
  ↓
Service
  ↓
Repository
  ↓
Models / Data
  ├── GORM 数据库模型
  ├── GORM / PostgreSQL
  └── go-redis / Redis
```

- `internal/modules/<domain>/api.go` 负责 Fiber 路由和 HTTP 转换
- `internal/modules/<domain>/service.go` 负责业务规则 流程和事务边界
- `internal/modules/<domain>/repository.go` 包含具体 Repository 手写查询和模型转换
- `internal/models` 只包含 GORM 数据库模型
- `internal/data` 只管理连接 事务和资源生命周期
- API 和 Service 不直接访问 GORM Redis 或 Data

## GORM Repository

数据库模型统一定义在 `internal/models`：

```go
package models

// User 映射业务用户表
type User struct {
	ID       string `gorm:"column:id;type:uuid;primaryKey"`
	Username string `gorm:"column:username;size:32;not null;uniqueIndex"`
}
```

每个业务模块只有一个具体 Repository，不创建 `Repository` 公开接口、私有实现或 `repository_gorm.go`：

```go

// Repository 使用 GORM 实现用户持久化
type Repository struct {
	data *data.Data
}

// NewRepository 创建用户 Repository
func NewRepository(resources *data.Data) *Repository {
	return &Repository{data: resources}
}

// FindByID 查询指定用户
func (r *Repository) FindByID(ctx context.Context, id string) (User, error) {
	var record models.User
	err := r.data.DB(ctx).
		Where("id = ?", id).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, NewNotFoundError()
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return toUser(record), nil
}
```

GORM 数据库模型只表达表映射，业务实体不携带 GORM 标签。密码哈希等敏感字段也不会因为复用数据库结构而进入 API 响应。

Repository 只实现当前业务需要的方法，不预建完整 CRUD，也不创建通用 Repository 基类。

## Service 的私有依赖接口

Service 使用具体结构体，不创建公开 Service 接口和私有实现：

```go
type repository interface {
	FindByID(ctx context.Context, id string) (User, error)
}

// Service 提供用户业务能力
type Service struct {
	repository repository
}

// NewService 创建用户服务
func NewService(storage repository) *Service {
	return &Service{repository: storage}
}

// Get 查询指定用户
func (s *Service) Get(ctx context.Context, id string) (User, error) {
	return s.repository.FindByID(ctx, id)
}
```

这里的私有 `repository` 接口属于 Service，用于描述它真正需要的方法并允许单元测试注入 Fake。它不是第二套持久层结构，生产环境仍然只有具体的 GORM Repository。

如果业务不需要 Fake，也可以让 Service 直接依赖 `*Repository`，但默认保留私有最小接口更利于独立测试业务规则。

## 手动装配

组合根显式创建具体 Repository 和 Service：

```go
userService := user.NewService(
	user.NewRepository(resources),
	passwords,
	appSessions,
)
user.RegisterHandlers(server.App(), userService, appRealm)
```

所有模块只在 `internal/app/modules.go` 装配一次，不使用 ServiceContext、运行时 DI 容器或包级全局依赖。

## Redis

业务缓存逻辑写在所属模块的 `repository.go`：

```go
func (r *Repository) cachedName(ctx context.Context, id string) (string, error) {
	key := "user:" + id + ":name"
	// 使用共享 Redis 客户端读取业务缓存
	return r.data.Redis().Get(ctx, key).Result()
}
```

缓存键和一致性策略留在业务模块，不放入 API、Service 或共享 Data。

## 事务

Service 决定事务边界，并通过私有最小接口依赖事务能力：

```go
type transactor interface {
	WithinTransaction(ctx context.Context, function func(context.Context) error) error
}

func (s *Service) Transfer(ctx context.Context, fromID string, toID string, amount int64) error {
	return s.transactor.WithinTransaction(ctx, func(transactionContext context.Context) error {
		if err := s.repository.Decrease(transactionContext, fromID, amount); err != nil {
			return err
		}
		return s.repository.Increase(transactionContext, toID, amount)
	})
}
```

Repository 必须继续传递收到的上下文，`Data.DB(ctx)` 会自动取得当前事务会话。Redis 不参与 PostgreSQL 事务，缓存更新和失效必须在事务成功返回后执行。

## 系统探针

系统探针不是业务模块，不经过 Service 和 Repository：

- `/healthz` 只检查 HTTP 进程
- `/readyz` 通过 `Data.Ping` 检查 PostgreSQL 和 Redis
- `/api/v1/ping` 通过 `Data.PingDatabase` 执行 PostgreSQL 轻量查询

探针路由统一放在 `internal/modules/health/api.go`，但不套用业务 Service 和 Repository。

## 数据库迁移

项目使用 `golang-migrate` v4 管理 PostgreSQL 版本，GORM 只负责模型映射、查询和业务事务。HTTP 服务启动不会调用 `AutoMigrate`，迁移必须作为独立命令显式执行。

目标 PostgreSQL 服务和逻辑数据库必须先存在。迁移器负责在目标数据库中创建业务表和 `schema_migrations` 版本表，不负责创建数据库、用户或权限。

创建下一组迁移：

```bash
go tool migrate create -ext sql -dir migrations -seq add_order_status
```

官方 CLI 按当前最大版本创建一组不覆盖已有文件的空迁移：

```text
migrations/000002_add_order_status.up.sql
migrations/000002_add_order_status.down.sql
```

升级、查看版本和单步回滚：

```bash
go run . migrate up -config configs/config.local.yaml -path migrations
go run . migrate version -config configs/config.local.yaml -path migrations
go run . migrate down -steps 1 -config configs/config.local.yaml -path migrations
```

`up` 在已经处于最新版本时视为成功。`version` 输出当前版本和 dirty 状态。`down` 必须明确指定步数，主要用于本地开发和测试。

需要直接使用带 PGX v5 驱动的官方 CLI 时传入 `pgx5://` 数据库 URL：

```bash
go run -tags pgx5 github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database '<pgx5-url>' up
go run -tags pgx5 github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database '<pgx5-url>' version
go run -tags pgx5 github.com/golang-migrate/migrate/v4/cmd/migrate -path migrations -database '<pgx5-url>' down 1
```

### 迁移编写规则

- 文件名使用六位递增版本、snake_case 名称和成对的 `up/down.sql`
- 默认使用 `BEGIN` 和 `COMMIT` 包裹同一版本中的多条 PostgreSQL 语句
- Up 和 Down 必须准确互逆，Down 同样需要考虑依赖对象的删除顺序
- `CREATE INDEX CONCURRENTLY` 等不能进入事务的语句单独放入一个迁移，并移除事务包装
- 模型字段、索引或约束变化必须同时更新 `internal/models` 和所属 Repository
- 已合并或已在共享环境执行的迁移不可修改、重命名、重排或删除，只能追加修复版本
- 删除列、收紧约束和不可逆数据转换采用 expand migrate contract 分阶段完成
- 应用回滚不会自动执行 Down，所以每个生产迁移至少兼容上一个应用版本
- dirty 状态表示迁移可能只执行了一部分，禁止自动 force，必须先人工核对真实结构

CI 使用真实 PostgreSQL 执行 `up -> down -> up`，覆盖迁移文件解析、版本记录、锁和回滚。部署阶段在旧应用仍运行时先执行 `migrate up`，失败则停止发布；迁移成功后才切换新应用。

## 安全约束

- 禁止 API 和 Service 直接调用 GORM 或 Redis
- 禁止使用包级全局连接
- 禁止在应用启动时调用 `AutoMigrate`
- 关联查询显式使用 `Preload` 或 `Joins`
- 更新时显式声明字段 避免使用 `Save` 覆盖未加载字段
- 动态排序字段和列名必须来自服务端白名单
- 原生 SQL 使用参数绑定
- GORM 日志保持参数化
