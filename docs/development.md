# 开发命令

项目通过 `tools/dev` 统一执行重复的开发操作

## 质量检查

```bash
go run ./tools/dev check
```

命令依次执行 `go fmt ./...` `go test ./...` `go vet ./...` 和 `go build ./...`

## 初始化迁移

优先使用仓库固定版本的官方 CLI：

```bash
go tool migrate create -ext sql -dir migrations -seq create_orders
```

项目开发工具也提供带事务模板和预览能力的等价初始化命令：

```bash
go run ./tools/dev migration new create_orders
```

工具扫描 `migrations` 中的最大版本并创建下一组 `up.sql` 和 `down.sql` 文件。模板默认使用 `BEGIN` 和 `COMMIT` 包裹多语句迁移。

迁移名称必须使用小写蛇形命名 预览时不会写入文件

```bash
go run ./tools/dev migration new create_orders --dry-run
```

补全 SQL 后通过正式迁移器执行和查看版本：

```bash
go run . migrate up -config configs/config.local.yaml -path migrations
go run . migrate version -config configs/config.local.yaml -path migrations
go run . migrate down -steps 1 -config configs/config.local.yaml -path migrations
```

默认配置中的数据库主机名用于 Compose 网络。在宿主机直接执行这些命令前，需要使用宿主机可访问的 PostgreSQL URL；也可以通过 `docker compose run --rm migrate` 在 Compose 网络内升级。

已合并或已经在共享环境执行的迁移文件不可修改，只能创建新版本修复。`CREATE INDEX CONCURRENTLY` 等不能在 PostgreSQL 事务中执行的语句必须单独创建迁移并移除事务包装。

## 初始化模块

```bash
go run ./tools/dev module new order --realm app
```

工具执行以下操作

- 创建 `internal/modules/order/api.go`
- 创建 `internal/modules/order/service.go`
- 创建 `internal/modules/order/repository.go`
- 根据 `app` `admin` 或 `none` 选择登录域
- 更新 `internal/app/modules.go`
- 格式化生成的 Go 源码

模块名只允许小写字母和数字且必须以字母开头 已存在的模块不会被覆盖

使用 `--dry-run` 查看全部计划内容

```bash
go run ./tools/dev module new order --realm admin --dry-run
```

初始化结果只提供可编译的模块结构 路由 业务方法 数据库模型 迁移和测试必须根据实际需求补充 Service 需要 Fake 时再将具体 Repository 依赖收窄为私有接口

## 自动化边界

开发命令只处理确定性的机械步骤 不生成通用 CRUD 不推导业务规则 不覆盖已有文件

后续 Skill 可以根据业务描述调用这些命令并继续完成模型 迁移 业务实现 测试和质量检查
