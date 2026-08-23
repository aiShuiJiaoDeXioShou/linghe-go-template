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
	writeTestFile(t, filepath.Join(migrationsDirectory, "000003_existing.up.sql"), "-- existing\n")
	writeTestFile(t, filepath.Join(migrationsDirectory, "000003_existing.down.sql"), "-- existing\n")

	var output bytes.Buffer
	err := devtool.Run(context.Background(), root, []string{"migration", "new", "create_orders"}, &output, &output)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, suffix := range []string{"up.sql", "down.sql"} {
		path := filepath.Join(migrationsDirectory, "000004_create_orders."+suffix)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated migration %s: %v", path, err)
		}
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
