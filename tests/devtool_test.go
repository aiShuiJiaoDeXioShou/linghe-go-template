package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-template/tools/devtool"
)

// TestDevtoolMigrationNew 验证迁移初始化器按最大版本递增并创建双向迁移
func TestDevtoolMigrationNew(t *testing.T) {
	root := newDevtoolFixture(t)
	migrationsDirectory := filepath.Join(root, "migrations")
	if err := os.MkdirAll(migrationsDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeTestFile(t, filepath.Join(migrationsDirectory, "000001_existing.up.sql"), "-- existing\n")
	writeTestFile(t, filepath.Join(migrationsDirectory, "000001_existing.down.sql"), "-- existing\n")

	var output bytes.Buffer
	err := devtool.Run(context.Background(), root, []string{"migration", "new", "create_orders"}, &output, &output)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, suffix := range []string{"up.sql", "down.sql"} {
		path := filepath.Join(migrationsDirectory, "000002_create_orders."+suffix)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated migration %s: %v", path, err)
		}
	}
}

// TestDevtoolMigrationCheck 验证迁移检查允许版本间隔但拒绝缺失配对
func TestDevtoolMigrationCheck(t *testing.T) {
	root := newDevtoolFixture(t)
	migrationsDirectory := filepath.Join(root, "migrations")
	writeTestFile(t, filepath.Join(migrationsDirectory, "000001_initial.up.sql"), "-- up\n")

	var output bytes.Buffer
	err := devtool.Run(context.Background(), root, []string{"migration", "check"}, &output, &output)
	if err == nil || !strings.Contains(err.Error(), "同时包含 up 和 down") {
		t.Fatalf("missing pair error = %v", err)
	}

	writeTestFile(t, filepath.Join(migrationsDirectory, "000001_initial.down.sql"), "-- down\n")
	writeTestFile(t, filepath.Join(migrationsDirectory, "000003_gap.up.sql"), "-- up\n")
	writeTestFile(t, filepath.Join(migrationsDirectory, "000003_gap.down.sql"), "-- down\n")
	err = devtool.Run(context.Background(), root, []string{"migration", "check"}, &output, &output)
	if err != nil {
		t.Fatalf("migration gap error = %v", err)
	}
}

// TestDevtoolMigrationDryRun 验证迁移预览不写入文件
func TestDevtoolMigrationDryRun(t *testing.T) {
	root := newDevtoolFixture(t)
	var output bytes.Buffer
	err := devtool.Run(
		context.Background(),
		root,
		[]string{"migration", "new", "create_orders", "--dry-run"},
		&output,
		&output,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output.String(), "000001_create_orders.up.sql") {
		t.Fatalf("dry-run output = %q", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, "migrations")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created migrations directory: %v", err)
	}
}

// TestDevtoolModuleNew 验证模块初始化器创建基础文件并更新组合根
func TestDevtoolModuleNew(t *testing.T) {
	root := newDevtoolFixture(t)
	writeModulesFixture(t, root)

	var output bytes.Buffer
	err := devtool.Run(
		context.Background(),
		root,
		[]string{"module", "new", "order", "--realm", "app"},
		&output,
		&output,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, name := range []string{"api.go", "service.go", "repository.go"} {
		path := filepath.Join(root, "internal", "modules", "order", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated module file %s: %v", path, err)
		}
	}
	modulesSource, err := os.ReadFile(filepath.Join(root, "internal", "app", "modules.go"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, expected := range []string{
		`ordermodule "example.com/project/internal/modules/order"`,
		`orderService := ordermodule.NewService(ordermodule.NewRepository(resources))`,
		`ordermodule.RegisterHandlers(router, orderService, realms.App)`,
		"ordermodule.RegisterHandlers(router, orderService, realms.App)\n\n\t// 注册不经过业务分层的系统探针\n\thealthmodule.RegisterHandlers",
	} {
		if !strings.Contains(string(modulesSource), expected) {
			t.Errorf("modules.go missing %q\n%s", expected, modulesSource)
		}
	}
}

// TestDevtoolModuleDryRunAndExistingTarget 验证模块预览不落盘且拒绝覆盖已有模块
func TestDevtoolModuleDryRunAndExistingTarget(t *testing.T) {
	root := newDevtoolFixture(t)
	writeModulesFixture(t, root)

	var output bytes.Buffer
	err := devtool.Run(
		context.Background(),
		root,
		[]string{"module", "new", "order", "--dry-run"},
		&output,
		&output,
	)
	if err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	moduleDirectory := filepath.Join(root, "internal", "modules", "order")
	if _, err := os.Stat(moduleDirectory); !os.IsNotExist(err) {
		t.Fatalf("dry-run created module directory: %v", err)
	}
	if err := os.MkdirAll(moduleDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	err = devtool.Run(context.Background(), root, []string{"module", "new", "order"}, &output, &output)
	if err == nil || !strings.Contains(err.Error(), "模块目录已存在") {
		t.Fatalf("existing module error = %v", err)
	}
}

// TestDevtoolProjectInitDryRun 验证项目初始化预览覆盖模块名和项目标识但不写入文件
func TestDevtoolProjectInitDryRun(t *testing.T) {
	root := newDevtoolFixture(t)
	writeTestFile(t, filepath.Join(root, "configs", "config.local.yaml"), "app:\n  name: old-service\n  env: local\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n\nimport _ \"example.com/project/internal/config\"\n")
	writeTestFile(t, filepath.Join(root, "docker-compose.yaml"), "name: old-service\nvolume: old_service_data\n")

	var output bytes.Buffer
	err := devtool.Run(context.Background(), root, []string{
		"project", "init",
		"--module", "example.com/order/service",
		"--name", "order-service",
		"--dry-run",
	}, &output, &output)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, expected := range []string{"example.com/project -> example.com/order/service", "old-service -> order-service", "update main.go", "update docker-compose.yaml"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("project dry-run missing %q\n%s", expected, output.String())
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "module example.com/project") {
		t.Fatalf("dry-run changed go.mod: %s", content)
	}
}

// TestDevtoolProjectInit 验证项目初始化统一更新模块 应用和数据库标识
func TestDevtoolProjectInit(t *testing.T) {
	root := newDevtoolFixture(t)
	writeTestFile(t, filepath.Join(root, "go.mod"), "module old-service\n\ngo 1.25\n")
	writeTestFile(t, filepath.Join(root, "configs", "config.local.yaml"), "app:\n  name: old-service\n  env: local\n")
	writeTestFile(t, filepath.Join(root, "configs", "config.stg.yaml"), "app:\n  name: old-service\n  env: stg\n")
	writeTestFile(t, filepath.Join(root, "configs", "config.production.yaml"), "app:\n  name: old-service\n  env: production\n")
	writeTestFile(t, filepath.Join(root, "internal", "sample", "sample.go"), "package sample\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n\nimport _ \"old-service/internal/sample\"\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "old-service old_service\n")

	var output bytes.Buffer
	err := devtool.Run(context.Background(), root, []string{
		"project", "init",
		"--module", "example.com/order/service",
		"--name", "order-service",
	}, &output, &output)
	if err != nil {
		t.Fatalf("Run() error = %v\n%s", err, output.String())
	}
	for path, expected := range map[string][]string{
		"go.mod":    {"module example.com/order/service"},
		"main.go":   {`"example.com/order/service/internal/sample"`},
		"README.md": {"order-service", "order_service"},
	} {
		content, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, readErr)
		}
		for _, value := range expected {
			if !strings.Contains(string(content), value) {
				t.Errorf("%s missing %q\n%s", path, value, content)
			}
		}
	}
}

// TestDevtoolReleaseDryRun 验证发布预览选择目标环境配置和稳定包名
func TestDevtoolReleaseDryRun(t *testing.T) {
	root := newDevtoolFixture(t)
	writeTestFile(t, filepath.Join(root, "configs", "config.stg.yaml"), "app:\n  name: order-service\n  env: stg\n")

	var output bytes.Buffer
	sha := strings.Repeat("a", 40)
	err := devtool.Run(context.Background(), root, []string{
		"release", "package",
		"--env", "stg",
		"--sha", sha,
		"--dry-run",
	}, &output, &output)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, expected := range []string{"order-service-stg-" + sha + ".tar.gz", "configs/config.stg.yaml"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("release dry-run missing %q\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "config.production.yaml") {
		t.Fatalf("release dry-run contains production config: %s", output.String())
	}
}

// TestDevtoolRejectsInvalidInput 验证初始化器拒绝非法名称和登录域
func TestDevtoolRejectsInvalidInput(t *testing.T) {
	root := newDevtoolFixture(t)
	testCases := []struct {
		name string
		args []string
	}{
		{name: "migration name", args: []string{"migration", "new", "CreateOrders"}},
		{name: "module name", args: []string{"module", "new", "order-item"}},
		{name: "realm", args: []string{"module", "new", "order", "--realm", "root"}},
		{name: "project module", args: []string{"project", "init", "--module", "invalid", "--name", "order"}},
		{name: "release env", args: []string{"release", "package", "--env", "local", "--sha", strings.Repeat("a", 40)}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := devtool.Run(context.Background(), root, testCase.args, &output, &output); err == nil {
				t.Fatalf("Run(%v) error = nil", testCase.args)
			}
		})
	}
}

func newDevtoolFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n\ngo 1.25\n")
	return root
}

func writeModulesFixture(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, "internal", "app", "modules.go"), `package app

import (
	"time"

	"example.com/project/internal/auth"
	"example.com/project/internal/data"
	healthmodule "example.com/project/internal/modules/health"

	"github.com/gofiber/fiber/v3"
)

func registerModules(
	router fiber.Router,
	resources *data.Data,
	realms *auth.Realms,
	readinessTimeout time.Duration,
) {
	// 注册不经过业务分层的系统探针
	healthmodule.RegisterHandlers(router, resources.Ping, resources.PingDatabase, readinessTimeout)
	// 注册认证接口
	auth.RegisterHandlers(router, realms)
}
`)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
