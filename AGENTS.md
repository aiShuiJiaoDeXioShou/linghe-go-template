# 项目协作规范

## 项目定位

- 本仓库默认作为可直接使用的私有项目模板
- 保持根目录 `main.go` 作为唯一进程入口 不移动到 `cmd` 目录
- 固定使用 Fiber v3 GORM v2 PostgreSQL go-redis SCS v2 goredisstore golang-migrate v4 和 Swag
- 优先使用标准库和少量必要依赖
- 不为尚未出现的业务预建目录 接口或抽象层

## 业务模块架构

- 业务代码按领域放在 `internal/modules/<domain>` 不按技术层创建全局横向目录
- 默认请求调用链为 `API -> Service -> Repository -> Models / Data`
- 每个业务模块默认使用 `api.go` `service.go` `repository.go` 三个文件
- `entity.go` `errors.go` 和按业务动作拆分的文件只在确有内容时创建
- `api.go` 通过 `RegisterHandlers` 注册路由并负责 HTTP 转换
- `service.go` 使用具体 `Service` 结构体承载业务规则 流程和事务边界
- `repository.go` 使用具体 `Repository` 结构体实现 GORM 查询和数据库模型转换
- `internal/models` 只存放 GORM 表映射 不包含查询 业务逻辑 HTTP DTO 连接或事务
- 数据库模型到业务实体的转换由所属 Repository 完成
- Service 和 Repository 构造函数返回具体指针 Service 可声明消费者所有的私有最小接口用于测试注入
- Service 对外方法签名禁止出现 Fiber GORM Redis 或 Data 类型
- `internal/data` 只管理 PostgreSQL Redis 事务和资源生命周期 不存放领域 Repository
- `internal/app` 是唯一组合根 新增模块在 `internal/app/modules.go` 显式装配一次
- `internal/modules/health` 直接注册系统探针 不套用业务 Service 和 Repository
- 禁止在 API 或 Service 中直接访问 GORM Redis 或 `Data.DB(ctx)`
- 禁止使用包级全局数据库或 Redis 客户端

## 通用工具

- 无业务语义的无状态通用函数放在 `internal/utils` 并按能力拆分文件
- 工具函数必须命名明确且只在形成项目级规则或真实复用时新增
- `internal/utils` 保持为叶子依赖 不承载业务校验 权限 错误映射 事务 HTTP 或数据访问
- 项目 UUID 统一通过 `utils.NewUUID` 生成 业务代码禁止直接调用第三方 UUID 库

## 路由与开发自动化

- Fiber 路由直接写在所属业务包的 `api.go`
- API 文档使用 `swaggo/swag` 从 Handler 注释生成 禁止由文档工具生成路由 DTO Service Repository 或数据库模型
- 每个公开 HTTP 接口保留最小 Swag 注释 `@Summary` `@Tags` `@ID` `@Success` 和 `@Router` 有参数时声明 `@Param` 受保护接口声明 `@Security BearerAuth`
- JSON 的 `@accept` 和 `@produce` 在 `main.go` 全局声明 `@Description` 只在 Summary 无法表达约束时使用 通用 `@Failure` 不在每个 Handler 重复声明
- `@ID` 是 Apifox 同步接口的稳定标识 已发布接口禁止随意修改
- 生成文档固定放在 `docs/swagger` 且禁止手工修改 修改路由 请求 响应或注释后执行 `go run ./tools/dev docs generate`
- `/docs/index.html` 和 `/docs/doc.json` 只在 `local` 和 `stg` 注册 `production` 禁止暴露 API 文档
- Apifox 使用 `stg` 环境的 `/docs/doc.json` 作为 Swagger URL 数据源
- 项目级重复操作统一收敛到 `tools/dev` 包括门禁 文档生成 项目初始化 模块初始化 迁移校验 集成测试和发布打包
- 新迁移通过 `go run ./tools/dev migration new <snake_case_name>` 创建 禁止手工猜测下一个版本号
- 提交前通过 `go run ./tools/dev migration check` 校验迁移命名 版本唯一性和方向配对 版本号允许存在间隔
- 初始化器只负责确定性的机械工作 包括创建用户明确选择的文件 更新 `internal/app/modules.go` 和执行格式化
- 初始化器产物必须可编译 模块骨架需要在交付前补齐实际业务内容
- 初始化器不得覆盖已有文件 目标已存在时必须明确失败或仅输出差异预览
- GORM 查询必须根据实际业务显式实现 禁止产生通用 CRUD 基类
- 开发命令用法参考 `docs/development.md`

## 参数校验与 HTTP 响应

- JSON 接口统一返回 `code` `message` `data` 三个字段
- 成功业务码固定为 `0` 并通过 `httpx.OK` 或 `httpx.Created` 返回
- 普通 JSON API 使用 `httpx.OK` 或 `httpx.Created` 流式响应 文件下载和无响应体接口除外
- 请求结构体使用 `validate` 标签 API 通过 `c.Bind()` 完成绑定和校验
- 绑定或校验错误直接返回给统一错误处理器 JSON 请求拒绝未知字段和多个连续值
- 参数格式错误使用业务码 `40000` 参数校验失败使用业务码 `40001`
- 失败业务码采用 `<HTTP 状态码><两位序号>` 格式 例如用户名冲突可以使用 `40901`
- 通用业务码定义在 `internal/apperror` 模块专用业务码定义在所属模块的 `errors.go`
- 使用 `apperror.Wrap` 保留底层原因 客户端不得收到 `err.Error()` SQL Redis 响应等内部信息
- `internal/httpx` 只负责 HTTP 绑定 校验 错误转换和响应写入

## 认证与权限

- 默认使用 `internal/auth` 提供的 SCS 服务端会话
- App 和 Admin 使用相互隔离的登录域 分别对应 `/api/**` 和 `/admin/**`
- Redis 键前缀包含项目 环境和登录域 且两个登录域不得相同
- 客户端令牌只允许通过 `Authorization: Bearer <token>` 传递 禁止从 Cookie 查询参数或请求体读取
- 登录接口为 `POST /api/auth/login` 和 `POST /admin/auth/login` 并先在所属 Service 校验凭据和账号状态
- 受保护路由使用对应登录域的 `AuthenticateMiddleware` 禁止 App 路由复用 Admin 中间件或反向复用
- API 使用 `auth.RequirePrincipal` 获取可信用户 ID 并显式传给业务函数
- 按注销范围使用 `LogoutCurrent` `LogoutDevice` 或 `Realms.LogoutUser`
- 原始令牌不得写入日志 错误响应 数据库或 Redis 键 Redis 中只保存令牌摘要和会话数据
- 敏感操作按需调用 `RequireRecentAuthentication` 不通过长期登录状态代替近期身份确认
- 权限默认由业务 Repository 查询数据库并实现 `auth.PermissionChecker` 管理端通过 `Authorizer` 校验权限码
- 密码使用 `golang.org/x/crypto` 提供的自适应哈希
- 登录 令牌轮换 跨域隔离 注销和权限拒绝必须具有测试
- 认证错误使用 `40101` 近期认证使用 `40102` 权限不足使用 `40301` 会话存储不可用使用 `50310` 权限数据不可用使用 `50311`

## 配置与私钥

- 配置放在 `configs` 并使用 YAML 只定义 `local` `stg` 和 `production` 三个环境
- PostgreSQL Redis 和连接池参数写入对应环境配置
- 仓库所有者允许提交项目配置和私钥 未经明确要求不得隐藏 删除 迁移或轮换

## 测试

- 所有 `*_test.go` 文件统一放在顶层 `tests` 目录
- Service 单元测试通过其私有最小依赖接口注入明确的 Fake
- 具体 GORM Repository 通过真实 PostgreSQL 集成测试验证

## 注释规范

- 所有人工编写的代码注释必须使用中文
- 注释禁止使用中文句号和中文分号
- 对外可调用的类型 函数和方法必须添加注释
- 私有函数根据重要程度添加注释 非复杂业务不用添加注释
- 注释应解释用途 约束或原因 避免简单复述代码
- Go 工具要求的 `//go:build` `//go:generate` `// Code generated ... DO NOT EDIT.` 和 Swag 指令可以保留标准英文格式

## 质量要求

- 修改 Go 文件后执行 `go fmt ./...` 并在交付前执行 `go run ./tools/dev check`
- 持久层改动必须在 CI 的真实 PostgreSQL 和 Redis 服务上执行集成测试

## 构造不变量与空值处理

- 必需依赖在组装或构造阶段校验并立即失败 构造成功后业务路径信任依赖非空
- 只有 nil 能合法到达且有对应测试时才增加防御性 nil 判断
- 禁止为不完整的测试夹具增加生产分支 测试应构造完整依赖或使用 Fake

## 数据层规范

- Repository 通过 `Data.DB(ctx)` 获取绑定上下文和当前事务的 GORM 会话
- Repository 通过 `Data.Redis()` 获取共享 Redis 客户端并继续传递请求上下文
- 跨数据操作事务由 Service 控制并继续传递收到的上下文
- 主键 审计字段和删除策略必须显式声明
- 应用服务启动不自动迁移 数据库结构变更使用 `migrations` 中的版本化 SQL
- GORM 的 `Preload` `Joins` 和更新字段必须显式声明
- 用户输入必须参数化 排序字段或列名必须经过白名单
- Redis 缓存只在数据库事务提交成功后更新或失效
- 详细示例参考 `docs/persistence.md`

## 数据库迁移

- 数据库迁移固定使用 `golang-migrate` v4 和 PGX v5 驱动 禁止使用 GORM `AutoMigrate`
- golang-migrate 只管理目标数据库中的结构和数据版本 不负责创建 PostgreSQL 服务 逻辑数据库 用户或权限
- 迁移文件使用六位递增版本和 snake_case 名称 每个版本必须同时包含 `<version>_<name>.up.sql` 和 `<version>_<name>.down.sql`
- Up 和 Down 必须准确互逆并默认使用 `BEGIN` `COMMIT` 保证 PostgreSQL 多语句原子执行
- `CREATE INDEX CONCURRENTLY` 等禁止在事务中执行的语句必须使用独立迁移并明确移除事务包装
- 已合并或已在任一共享环境执行的迁移文件禁止修改 重命名 重排或删除 修复只能追加新版本
- 数据库字段 表 索引 约束或数据语义变化必须在同一变更中同步迁移 SQL `internal/models` 和 Repository
- 破坏性变更必须采用 expand migrate contract 分批发布并至少兼容上一个应用版本
- CI 必须在真实 PostgreSQL 上通过 `up -> down -> up` 并确认版本不是 dirty
- 发布包存在迁移时必须先执行 `migrate up` 再启动新应用 迁移失败时保留旧应用且禁止自动 force 没有迁移时允许直接发布
- 应用回滚只恢复上一版应用 数据库禁止自动执行 down 生产修复默认追加向前迁移
- `migrate down` 主要用于本地和测试 生产执行 down 或 force 前必须人工核对数据库真实状态和数据影响

## 分支与部署

- `main` push 自动部署到 `stg` `rel` push 自动部署到 `production` 其他分支不自动部署
- CI 和部署前必须通过测试 静态检查和构建
- 远端部署只更新后端容器 不自动创建或覆盖 PostgreSQL 和 Redis
- 远端部署在旧版本继续服务时构建镜像并执行迁移 迁移成功后才切换应用
- 部署后使用 `/readyz` 检查依赖状态 失败时只恢复上一个应用版本且不回滚数据库
- 具体参数以 `.github/workflows` 和 `deploy` 中的当前文件为准
