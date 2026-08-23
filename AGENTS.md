# 项目协作规范

## 项目定位

- 本仓库默认作为可直接使用的私有项目模板
- 项目采用按业务模块内聚的轻量单体架构
- 保持根目录 `main.go` 作为唯一进程入口 不移动到 `cmd` 目录
- HTTP 框架使用 Fiber v3
- PostgreSQL 持久层框架使用 GORM v2 和 PostgreSQL 驱动
- Redis 客户端使用 go-redis
- 登录会话使用 SCS v2 和 goredisstore
- 优先使用标准库和少量必要依赖
- 不为尚未出现的业务预建目录 接口或抽象层

## 业务模块架构

- 业务代码按领域放在 `internal/modules/<domain>` 不按技术层创建全局横向目录
- 默认请求调用链为 `API -> Service -> Repository -> Models / Data`
- 每个业务模块默认使用 `api.go` `service.go` `repository.go` 三个文件
- `entity.go` `errors.go` 和按业务动作拆分的文件只在确有内容时创建
- `api.go` 通过 `RegisterHandlers` 注册 Fiber 路由并完成请求解析 参数校验 错误映射和响应转换
- `service.go` 使用具体 `Service` 结构体集中承载业务规则 业务流程和事务边界
- `repository.go` 使用具体 `Repository` 结构体实现手写查询和数据库模型转换
- `internal/models` 只存放 GORM 数据库模型 字段标签 表名 索引和关联映射
- 数据库模型必须集中定义在 `internal/models` 禁止散落在业务模块的 `repository.go`
- `internal/models` 禁止包含查询 业务逻辑 HTTP DTO 数据库连接和事务控制
- `internal/models` 不得依赖 `internal/modules` 数据库模型到业务实体的转换由 Repository 完成
- Service 和 Repository 构造函数返回具体指针 禁止创建公开接口加私有实现的固定模板
- 项目固定使用 GORM 不创建 `repository_gorm.go` 或其他持久层多实现文件
- Service 可以在 `service.go` 内声明消费者所有的私有最小接口用于测试注入
- 私有依赖接口只包含当前 Service 实际调用的方法 不表示存在多套持久层实现
- Service 对外方法签名禁止出现 Fiber GORM Redis 或 Data 类型
- Repository 禁止创建通用 CRUD 基类
- `internal/data` 只管理 PostgreSQL Redis 事务和资源生命周期 不存放领域 Repository
- `internal/health` 直接注册系统探针 不套用业务 Service 和 Repository
- `internal/httpserver` 只管理 Fiber 实例 中间件 错误处理和网络生命周期
- `internal/app` 是唯一组合根 负责配置加载 手动依赖注入 路由注册 启停顺序和资源释放
- 新增模块时在 `internal/app/modules.go` 增加一次显式装配
- 禁止重新创建全局 `internal/handler` `internal/logic` `internal/model` `internal/types` `internal/svc` `internal/transport`
- 禁止使用 ServiceContext 或其他传入全部业务依赖的服务定位器
- 禁止在 API 或 Service 中直接访问 GORM Redis 或 `Data.DB(ctx)`
- 禁止使用包级全局数据库或 Redis 客户端

## 路由与代码生成

- Fiber 路由直接写在所属业务包的 `api.go`
- 当前模板不使用路由清单 IDL 或脚手架生成 API Service Repository
- 不创建仅包含 method 和 path 的顶层 `api` 目录
- 未来只有引入完整 OpenAPI Proto 或 Thrift 契约时才重新建立 `api` 或 `idl` 目录
- Repository 的 GORM 数据结构 CRUD 和自定义查询全部手写 禁止生成持久层代码
- 数据库结构必须在 `internal/models` 中显式声明字段和 GORM 标签

## 参数校验与 HTTP 响应

- JSON 接口统一返回 `code` `message` `data` 三个字段
- 成功业务码固定为 `0` 并通过 `httpx.OK` 或 `httpx.Created` 返回
- 禁止在普通 JSON API 中直接调用 `c.JSON` 或 `c.Status(...).JSON(...)`
- 流式响应 文件下载和无响应体接口不受统一 JSON 结构限制
- 请求结构体使用 `validate` 标签声明校验规则
- API 使用 `c.Bind().Body` `c.Bind().Query` 或 `c.Bind().URI` 完成绑定并自动校验
- API 遇到绑定或校验错误时直接返回错误 禁止自行拼接字段错误响应
- JSON 请求默认严格解析并拒绝未知字段和多个连续 JSON 值
- 参数格式错误使用业务码 `40000` 参数校验失败使用业务码 `40001`
- 失败业务码采用 `<HTTP 状态码><两位序号>` 格式 例如用户名冲突可以使用 `40901`
- 通用业务码统一定义在 `internal/apperror` 模块专用业务码定义在所属 feature 的 `errors.go`
- 需要保留底层原因时使用 `apperror.Wrap` 客户端只返回安全消息和明确允许的详情
- 禁止将 `err.Error()` SQL Redis 响应或其他内部错误直接返回客户端
- `internal/httpx` 只负责 HTTP 绑定 校验 错误转换和响应写入 不承载业务规则
- Service 不依赖 Fiber 或 `internal/httpx` 业务错误可以使用 `internal/apperror`

## 认证与权限

- 默认使用 `internal/auth` 提供的 SCS 服务端会话
- App 和 Admin 必须使用两个相互隔离的登录域 分别对应 `/api/**` 和 `/admin/**`
- 两个登录域可以使用同一套用户数据 但令牌 Redis 会话和反向索引不得互通
- Redis 键前缀必须同时包含项目 环境和登录域 并保证 App 与 Admin 前缀不同
- 客户端令牌只允许通过 `Authorization: Bearer <token>` 传递 禁止从 Cookie 查询参数或请求体读取
- 登录接口必须先在所属业务 Service 中完成密码 短信或第三方凭据校验 再签发对应登录域会话
- 业务用户登录使用 `POST /api/auth/login` 管理员登录使用 `POST /admin/auth/login`
- 受保护路由使用对应登录域的 `AuthenticateMiddleware` 禁止 App 路由复用 Admin 中间件或反向复用
- API 和 Service 使用 `auth.RequirePrincipal` 获取服务端会话中的可信用户 ID 禁止相信客户端提交的用户 ID
- 业务函数需要当前用户时应显式传递 `Principal` 或用户 ID 禁止在深层代码中反复读取 Fiber 上下文
- 当前会话注销使用 `LogoutCurrent` 指定设备下线使用 `LogoutDevice` 用户禁用 密码重置或权限重大变化后使用 `Realms.LogoutUser`
- 原始令牌不得写入日志 错误响应 数据库或 Redis 键 Redis 中只保存令牌摘要和会话数据
- 敏感操作按需调用 `RequireRecentAuthentication` 不通过长期登录状态代替近期身份确认
- 权限默认由业务 Repository 查询数据库并实现 `auth.PermissionChecker` 管理端通过 `Authorizer` 校验权限码
- 只有出现复杂角色继承 多租户域或策略关系后才引入 Casbin 不为简单权限表提前增加策略引擎
- 禁止使用包级全局认证对象或类似 `StpUtil` 的隐式门面 登录域通过构造函数显式注入
- 密码落库时必须使用 `golang.org/x/crypto` 提供的自适应密码哈希 禁止明文 MD5 SHA1 或普通 SHA256
- 登录 令牌轮换 跨域隔离 按设备注销 按用户注销和权限拒绝必须具有测试
- 认证错误使用 `40101` 近期认证使用 `40102` 权限不足使用 `40301` 会话存储不可用使用 `50310` 权限数据不可用使用 `50311`

## 配置与私钥

- 应用配置统一放在 `configs` 目录
- 配置文件默认使用 YAML 格式
- 仓库只定义 `local` `stg` 和 `production` 三个运行环境
- 本地环境使用 `configs/config.local.yaml`
- 预发布环境使用 `configs/config.stg.yaml`
- 生产环境使用 `configs/config.production.yaml`
- PostgreSQL 和 Redis 的地址 账号 密码及连接池参数统一写入对应 YAML
- 不使用 `.env` 文件管理应用配置
- 配置 模板 静态文件和迁移文件应按用途分别建立目录 不使用笼统的 `res` 目录
- 仓库所有者明确允许将项目配置和私钥直接提交到 Git
- 私钥无需隐藏 脱敏 替换为占位符或迁移到环境变量
- 不要默认将私钥或包含私钥的配置文件加入 `.gitignore`
- 不要仅因为文件包含私钥就阻止提交或删除对应内容
- 只有用户明确要求时才对私钥进行隐藏 迁移 删除或轮换
- 仓库所有者明确接受私钥进入 Git 历史和已有克隆的结果

## 目录与测试

- 业务代码放在根目录 `main.go` 和 `internal/modules` 目录
- 新业务直接创建 `internal/modules/<domain>` 目录 不增加无业务含义的中间目录
- GORM 数据库模型统一放在 `internal/models` 目录
- 数据库版本迁移按需放在 `migrations` 目录
- 部署所需的镜像 Compose 配置和运维脚本放在 `deploy` 目录
- 项目开发工具按需放在 `tools` 目录 空目录不保留
- 所有 `*_test.go` 文件统一放在顶层 `tests` 目录
- 业务代码目录中不放置测试文件
- 不使用 `_test` 目录 因为 Go 工具会忽略以下划线开头的目录
- Service 单元测试通过其私有最小依赖接口注入明确的 Fake
- 具体 GORM Repository 通过真实 PostgreSQL 集成测试验证
- 如需跨包测试只提供最小且合理的公开入口

## 注释规范

- 所有人工编写的代码注释必须使用中文
- 注释中禁止使用中文句号 `。` 和中文分号 `；`
- 注释句末不得使用中文句号 `。`
- 尽量保持代码和注释简洁
- 内部函数调用点应添加中文注释并说明调用目的
- 对外可调用的类型 函数和方法必须添加注释
- 私有函数根据重要程度添加注释 非复杂业务不用添加注释
- 工具类型和工具函数应尽可能添加注释
- 注释应解释用途 约束或原因 避免简单复述代码
- Go 工具要求的 `//go:build` `//go:generate` 和 `// Code generated ... DO NOT EDIT.` 可以保留标准英文格式

## 质量要求

- 修改 Go 文件后执行 `gofmt`
- 提交前执行 `go test ./...`
- 提交前执行 `go vet ./...`
- 提交前执行 `go build ./...`
- 保持 `go.mod` 和 `go.sum` 与源码依赖一致
- 持久层改动必须在 CI 的真实 PostgreSQL 和 Redis 服务上执行集成测试

## 构造不变量与空值处理

- 必需依赖只在应用组装或构造函数中校验一次
- 构造成功后 API Service Repository 必须信任必需依赖非空
- 禁止对构造注入的必需依赖 Receiver 和内部字段添加防御性 nil 判断
- 新增上述 nil 判断前必须证明 nil 能通过合法运行路径到达并提供对应测试
- 必需依赖缺失时必须在启动或构造阶段立即失败 禁止在业务路径中降级为 HTTP 错误
- 禁止为了支持不完整的测试夹具而增加生产代码分支
- 测试必须构造完整依赖或使用明确的 Fake
- 这些限制不适用于 `error` 外部输入 可选依赖 map 查询 类型断言和明确声明可空的 API 返回值

## Code Review Rules

- 检查新增的依赖 nil 判断是否具有合法可达路径
- 如果依赖由构造函数保证非空 应删除业务层中的防御性 nil 判断
- 如果 nil 确实是合法状态 应在类型或构造函数契约中明确说明
- 检查一个业务变更是否收拢在所属 `internal/modules/<domain>` 模块
- 检查 GORM 数据库模型是否集中在 `internal/models` 且不包含查询和业务逻辑
- 检查 Service 是否包含持久化细节以及 Repository 是否包含业务流程
- 检查 API 是否绕过 Service 直接调用 Repository
- 检查是否为 Service 或 Repository 创建了无意义的公开接口和私有实现
- 检查系统探针等非业务功能是否被错误套入完整业务分层

## 数据层规范

- 应用启动时必须建立 PostgreSQL 和 Redis 连接 连接失败时直接终止启动
- `/healthz` 只用于检查 HTTP 进程是否存活
- `/readyz` 必须同时检查 PostgreSQL 和 Redis 是否可用
- `/api/v1/ping` 通过 `Data.PingDatabase` 执行 PostgreSQL 轻量查询验证业务数据库连接
- Repository 通过 `Data.DB(ctx)` 获取绑定上下文和当前事务的 GORM 会话
- Repository 通过 `Data.Redis()` 获取共享 Redis 客户端并继续传递请求上下文
- 跨数据操作事务由 Service 使用所属文件内定义的私有窄接口控制
- 事务函数必须继续传递收到的上下文 不得自行创建后台上下文
- 不直接嵌入 `gorm.Model` 主键 审计字段和删除策略应显式声明
- 生产环境禁止在应用启动时调用 `AutoMigrate`
- 数据库结构变更必须使用 `migrations` 目录中的版本化 SQL
- GORM 的 `Preload` `Joins` 和更新字段必须显式声明
- 禁止使用字符串拼接生成包含用户输入的 SQL 排序字段或列名必须经过白名单
- GORM 日志默认参数化 避免查询参数进入日志
- Redis 缓存只在数据库事务提交成功后更新或失效
- Repository CRUD 和业务查询必须手写 不引入 CRUD 生成器
- 详细示例参考 `docs/persistence.md`

## 分支与部署

- `local` 只用于本地开发 不配置自动部署
- `main` 是主分支 每次 push 自动部署到 `stg` 环境
- `rel` 是生产发布分支 每次 push 自动部署到 `production` 环境
- 其他分支不得自动部署
- Pull Request 通过 CI 执行测试和静态检查
- 自动部署必须先执行 `go test ./...` `go vet ./...` 和构建检查
- GitHub Environments 使用 `stg` 和 `production` 两个名称
- 两个环境分别配置 `SSH_HOST` `SSH_USER` `SSH_PORT` `SSH_HOST_FINGERPRINT` `DEPLOY_DIR` `APP_PORT` Variables
- SSH 密码默认使用 `SSH_PASSWORD` Secret 用户提供私钥后也允许将私钥文件直接提交到仓库
- 远端部署目录必须位于 `/opt` 下 预发布和生产必须使用不同目录
- 远端 Docker 网络默认使用 `1panel-network`
- PostgreSQL 和 Redis 在远端网络中的服务名分别使用 `postgresql` 和 `redis`
- 远端部署只更新后端容器 不自动创建或覆盖 PostgreSQL 和 Redis
- 部署后使用 `/readyz` 检查依赖状态 失败时恢复上一个成功版本
