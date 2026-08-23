package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go-template/internal/config"
)

// TestLoadConfig 验证 YAML 配置加载
func TestLoadConfig(t *testing.T) {
	// 创建包含完整配置的临时文件
	path := writeConfig(t, `
app:
  name: example
  env: production
http:
  host: 127.0.0.1
  port: 8080
  read_timeout: 2s
  write_timeout: 3s
  idle_timeout: 30s
  shutdown_timeout: 5s
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "example" {
		t.Errorf("App.Name = %q, want %q", cfg.App.Name, "example")
	}
	if cfg.App.Env != "production" {
		t.Errorf("App.Env = %q, want %q", cfg.App.Env, "production")
	}
	if cfg.HTTP.Address() != "127.0.0.1:8080" {
		t.Errorf("HTTP.Address() = %q, want %q", cfg.HTTP.Address(), "127.0.0.1:8080")
	}
	if cfg.HTTP.ReadTimeout != 2*time.Second {
		t.Errorf("ReadTimeout = %s, want %s", cfg.HTTP.ReadTimeout, 2*time.Second)
	}
	if cfg.PostgreSQL.MaxOpenConnections != 10 {
		t.Errorf("PostgreSQL.MaxOpenConnections = %d, want %d", cfg.PostgreSQL.MaxOpenConnections, 10)
	}
	if cfg.Redis.Address != "redis:6379" {
		t.Errorf("Redis.Address = %q, want %q", cfg.Redis.Address, "redis:6379")
	}
	if cfg.Auth.App.KeyPrefix != "go-template:local:auth:app:" {
		t.Errorf("Auth.App.KeyPrefix = %q, want %q", cfg.Auth.App.KeyPrefix, "go-template:local:auth:app:")
	}
	if cfg.Auth.Admin.KeyPrefix != "go-template:local:auth:admin:" {
		t.Errorf("Auth.Admin.KeyPrefix = %q, want %q", cfg.Auth.Admin.KeyPrefix, "go-template:local:auth:admin:")
	}
}

// TestRepositoryConfigs 验证仓库内三环境配置可以加载
func TestRepositoryConfigs(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repositoryRoot := filepath.Dir(filepath.Dir(currentFile))

	tests := []struct {
		path            string
		wantEnvironment string
		wantAppPrefix   string
		wantAdminPrefix string
	}{
		{
			path:            "configs/config.local.yaml",
			wantEnvironment: "local",
			wantAppPrefix:   "go-template:local:auth:app:",
			wantAdminPrefix: "go-template:local:auth:admin:",
		},
		{
			path:            "configs/config.stg.yaml",
			wantEnvironment: "stg",
			wantAppPrefix:   "go-template:stg:auth:app:",
			wantAdminPrefix: "go-template:stg:auth:admin:",
		},
		{
			path:            "configs/config.production.yaml",
			wantEnvironment: "production",
			wantAppPrefix:   "go-template:production:auth:app:",
			wantAdminPrefix: "go-template:production:auth:admin:",
		},
	}
	for _, test := range tests {
		t.Run(test.wantEnvironment, func(t *testing.T) {
			// 加载仓库配置并核对环境映射
			cfg, err := config.Load(filepath.Join(repositoryRoot, filepath.FromSlash(test.path)))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.App.Env != test.wantEnvironment {
				t.Errorf("App.Env = %q, want %q", cfg.App.Env, test.wantEnvironment)
			}
			if cfg.Auth.App.KeyPrefix != test.wantAppPrefix {
				t.Errorf("Auth.App.KeyPrefix = %q, want %q", cfg.Auth.App.KeyPrefix, test.wantAppPrefix)
			}
			if cfg.Auth.Admin.KeyPrefix != test.wantAdminPrefix {
				t.Errorf("Auth.Admin.KeyPrefix = %q, want %q", cfg.Auth.Admin.KeyPrefix, test.wantAdminPrefix)
			}
		})
	}
}

// TestLoadConfigDefaults 验证空 YAML 使用默认配置
func TestLoadConfigDefaults(t *testing.T) {
	// 创建空对象配置用于验证默认值
	path := writeConfig(t, "{}")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "go-template" {
		t.Errorf("App.Name = %q, want %q", cfg.App.Name, "go-template")
	}
	if cfg.App.Env != "local" {
		t.Errorf("App.Env = %q, want %q", cfg.App.Env, "local")
	}
	if cfg.HTTP.Address() != "0.0.0.0:3000" {
		t.Errorf("HTTP.Address() = %q, want %q", cfg.HTTP.Address(), "0.0.0.0:3000")
	}
	if cfg.HTTP.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %s, want %s", cfg.HTTP.ShutdownTimeout, 10*time.Second)
	}
	if cfg.Auth.SessionLifetime != 30*24*time.Hour {
		t.Errorf("Auth.SessionLifetime = %s, want %s", cfg.Auth.SessionLifetime, 30*24*time.Hour)
	}
	if cfg.Auth.IdleTimeout != 7*24*time.Hour {
		t.Errorf("Auth.IdleTimeout = %s, want %s", cfg.Auth.IdleTimeout, 7*24*time.Hour)
	}
}

// TestLoadConfigRejectsInvalidPort 验证非法端口会被拒绝
func TestLoadConfigRejectsInvalidPort(t *testing.T) {
	// 创建包含非法端口的临时配置
	path := writeConfig(t, "http:\n  port: 70000")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want an invalid port error")
	}
}

// TestLoadConfigRejectsUnknownField 验证未知字段会被拒绝
func TestLoadConfigRejectsUnknownField(t *testing.T) {
	// 创建包含拼写错误字段的临时配置
	path := writeConfig(t, "http:\n  prot: 3000")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want an unknown field error")
	}
}

// TestLoadConfigRejectsUnknownEnvironment 验证未定义环境会被拒绝
func TestLoadConfigRejectsUnknownEnvironment(t *testing.T) {
	// 创建包含未定义环境的临时配置
	path := writeConfig(t, "app:\n  env: development")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want an invalid environment error")
	}
}

// TestLoadConfigRejectsUnknownGORMLogLevel 验证非法 GORM 日志级别会被拒绝
func TestLoadConfigRejectsUnknownGORMLogLevel(t *testing.T) {
	// 创建包含非法日志级别的临时配置
	path := writeConfig(t, "postgresql:\n  log_level: debug")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want an invalid GORM log level error")
	}
}

// TestLoadConfigRejectsSharedAuthPrefix 验证两个登录域不能共享 Redis 键空间
func TestLoadConfigRejectsSharedAuthPrefix(t *testing.T) {
	path := writeConfig(t, "auth:\n  app:\n    key_prefix: shared\n  admin:\n    key_prefix: shared")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want a duplicated auth prefix error")
	}
}

// TestLoadConfigRejectsLongAuthIdleTimeout 验证空闲时限不能超过会话有效期
func TestLoadConfigRejectsLongAuthIdleTimeout(t *testing.T) {
	path := writeConfig(t, "auth:\n  session_lifetime: 1h\n  idle_timeout: 2h")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() error = nil, want an invalid auth idle timeout error")
	}
}

// writeConfig 创建临时 YAML 配置文件
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
