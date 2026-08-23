# 开发命令

项目通过 `tools/dev` 统一执行确定性的重复操作

```bash
go run ./tools/dev help
```

## 命令概览

| 命令 | 用途 |
| --- | --- |
| `check` | 执行测试 静态检查和构建 |
| `docs generate` | 从 Handler 注释生成 Swagger 文档 |
| `project init` | 初始化模块路径和项目标识 |
| `module new` | 创建最小业务模块骨架并完成装配 |
| `migration new` | 创建下一组版本化迁移 |
| `migration check` | 校验迁移命名 版本唯一性和方向配对 |
| `test integration` | 使用本地 Compose 依赖执行集成测试 |
| `release package` | 生成指定环境的 Linux 发布包 |

## 质量检查

开发过程中主动格式化源码

```bash
go fmt ./...
```

提交前执行基础门禁

```bash
go run ./tools/dev check
```

`check` 执行 `go test ./...` `go vet ./...` 和 `go build ./...`，用于拦截测试失败 静态错误和编译失败

格式 迁移 Compose 和发布结构由各自的专项命令负责 CI 和自动部署统一调用 `check`

## API 文档

修改路由 请求 响应或 Swag 注释后重新生成文档

```bash
go run ./tools/dev docs generate
```

生成文件固定放在 `docs/swagger` 并提交到 Git 禁止手工修改

Handler 默认只保留 `Summary/Tags/ID/Param/Success/Security/Router` JSON 输入输出格式在 `main.go` 全局声明 通用错误结构不在每个接口重复标注

应用使用 `local` 或 `stg` 配置启动后可访问

```text
/docs/index.html
/docs/doc.json
```

`production` 环境不注册文档路由 Apifox 使用 `stg` 的 `/docs/doc.json` 作为 Swagger URL 数据源

## 初始化真实项目

从模板创建项目后先预览替换范围

```bash
go run ./tools/dev project init \
  --module github.com/example/order-service \
  --name order-service \
  --dry-run
```

确认后执行初始化

```bash
go run ./tools/dev project init \
  --module github.com/example/order-service \
  --name order-service
```

命令会统一更新

- `go.mod` 模块路径和全部项目内 imports
- 应用名 Docker 镜像名和发布包名
- PostgreSQL Redis 测试标识和会话键前缀
- 配置 文档 工作流和部署脚本中的模板标识
- 下划线形式的数据库资源名

初始化完成后自动执行 `go mod tidy` `go fmt ./...` Swagger 刷新和 `check`

项目目录和 Git 远端不会自动修改 数据库地址 账号 密码和 GitHub Environment 变量仍需根据实际环境确认

## 初始化模块

```bash
go run ./tools/dev module new order --realm app
```

工具会创建 `api.go` `service.go` `repository.go`，并更新 `internal/app/modules.go`

`--realm` 只允许 `app` `admin` 或 `none` 模块名只允许小写字母和数字 已存在的模块不会被覆盖

```bash
go run ./tools/dev module new order --realm admin --dry-run
```

模块骨架不生成模型 CRUD DTO 或迁移 这些内容必须根据真实业务补充

## 数据库迁移

创建下一组迁移

```bash
go run ./tools/dev migration new create_orders
```

预览但不写入文件

```bash
go run ./tools/dev migration new create_orders --dry-run
```

创建前会先检查已有迁移，并基于当前最大版本生成下一组 `up.sql` 和 `down.sql`

单独检查全部迁移

```bash
go run ./tools/dev migration check
```

执行迁移和查看数据库版本继续使用应用进程入口

```bash
go run . migrate up -config configs/config.local.yaml -path migrations
go run . migrate version -config configs/config.local.yaml -path migrations
go run . migrate down -steps 1 -config configs/config.local.yaml -path migrations
```

默认配置使用 Compose 服务名 在宿主机直接执行前需要确保 PostgreSQL 地址可访问

## 集成测试

```bash
go run ./tools/dev test integration
```

命令会

- 启动本地 PostgreSQL 和 Redis 容器并等待健康
- 根据本地项目名重建以 `_test` 结尾的隔离数据库
- 设置测试连接参数并执行 `go test ./...`

PostgreSQL 和 Redis 容器会保留运行以便继续开发 测试数据库不会用于应用本地数据

## 发布打包

预览发布内容

```bash
go run ./tools/dev release package \
  --env stg \
  --sha "$(git rev-parse HEAD)" \
  --goarch amd64 \
  --dry-run
```

生成发布包

```bash
go run ./tools/dev release package \
  --env stg \
  --sha "$(git rev-parse HEAD)" \
  --goarch amd64
```

默认输出到 `dist/<project>-<env>-<sha>.tar.gz` 也可以通过 `--output` 指定完整归档路径

发布命令会校验目标配置 迁移和 Compose，构建静态 Linux 二进制，并只打包目标环境配置

发布包包含

```text
server
Dockerfile
docker-compose.yml
release.json
configs/config.<env>.yaml
migrations/*.sql                # 存在迁移时包含
```

`release package` 只负责生成可部署归档 上传 SSH 切换版本 健康检查和应用回滚仍由部署工作流处理 没有迁移文件时远端会跳过数据库升级

## 自动化边界

- 工具只执行确定性的项目级操作
- 生成器不覆盖已有文件
- 不生成通用 CRUD Repository 或业务规则
- 不自动创建远端 PostgreSQL Redis 用户或权限
- 不包装简单的 Docker Compose Git 和应用迁移命令
