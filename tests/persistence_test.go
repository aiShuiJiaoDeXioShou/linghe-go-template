package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-template/internal/config"
	"go-template/internal/data"
	"go-template/internal/httpserver"
	adminusermodule "go-template/internal/modules/adminuser"
	configmodule "go-template/internal/modules/config"
	usermodule "go-template/internal/modules/user"

	"gorm.io/gorm"
)

const persistenceProbeTable = "go_template_persistence_probe"

// TestDataResources 验证统一数据资源和事务能力
func TestDataResources(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	redisAddress := os.Getenv("TEST_REDIS_ADDRESS")
	if databaseURL == "" || redisAddress == "" {
		t.Skip("TEST_POSTGRES_URL and TEST_REDIS_ADDRESS are not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建真实 PostgreSQL 和 Redis 资源
	resources, err := data.Open(ctx, config.Config{
		PostgreSQL: config.PostgreSQL{
			URL:                   databaseURL,
			MaxOpenConnections:    4,
			MaxIdleConnections:    2,
			ConnectionMaxLifetime: time.Minute,
			ConnectionMaxIdleTime: time.Minute,
			ConnectTimeout:        5 * time.Second,
			SlowQueryThreshold:    time.Second,
			LogLevel:              "silent",
		},
		Redis: config.Redis{
			Address:      redisAddress,
			Password:     os.Getenv("TEST_REDIS_PASSWORD"),
			Database:     0,
			PoolSize:     4,
			MinIdleConns: 1,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("data.Open() error = %v", err)
	}
	applyBusinessMigration(t, ctx, resources, "000001_create_core_business_tables.up.sql")
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = resources.DB(cleanupContext).Exec("DROP TABLE IF EXISTS " + persistenceProbeTable).Error
		applyBusinessMigration(t, cleanupContext, resources, "000001_create_core_business_tables.down.sql")
		_ = resources.Close()
	})

	t.Run("transaction and error translation", func(t *testing.T) {
		testTransactionAndErrorTranslation(t, ctx, resources)
	})
	t.Run("redis connection", func(t *testing.T) {
		if err := resources.Redis().Ping(ctx).Err(); err != nil {
			t.Fatalf("Redis Ping() error = %v", err)
		}
	})
	t.Run("ping route", func(t *testing.T) {
		testPingDatabaseRoute(t, resources)
	})
	t.Run("business repositories", func(t *testing.T) {
		testBusinessRepositories(t, ctx, resources)
	})
}

// testPingDatabaseRoute 验证 Ping 接口执行数据库连接检查
func testPingDatabaseRoute(t *testing.T, resources *data.Data) {
	t.Helper()
	server := newTestServer(httpserver.Config{AppName: "test"}, resources.PingDatabase, resources.Ping)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var body responseEnvelope[struct {
		Message  string `json:"message"`
		Database string `json:"database"`
	}]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Message != "pong" || body.Data.Database != "ok" {
		t.Errorf("response = %+v, want message pong and database ok", body.Data)
	}
}

// testTransactionAndErrorTranslation 验证事务回滚和错误转换
func testTransactionAndErrorTranslation(t *testing.T, ctx context.Context, resources *data.Data) {
	t.Helper()

	// 创建隔离的数据资源探针表
	if err := resources.DB(ctx).Exec(`CREATE TABLE IF NOT EXISTS ` + persistenceProbeTable + ` (
		id BIGINT PRIMARY KEY,
		value TEXT NOT NULL UNIQUE
	)`).Error; err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	if err := resources.DB(ctx).Exec("TRUNCATE TABLE " + persistenceProbeTable).Error; err != nil {
		t.Fatalf("truncate probe table: %v", err)
	}

	// 返回业务错误触发事务回滚
	rollbackError := errors.New("rollback transaction")
	err := resources.WithinTransaction(ctx, func(transactionContext context.Context) error {
		if err := resources.DB(transactionContext).
			Exec("INSERT INTO "+persistenceProbeTable+" (id, value) VALUES (?, ?)", 1, "rollback").Error; err != nil {
			return err
		}
		return rollbackError
	})
	if !errors.Is(err, rollbackError) {
		t.Fatalf("WithinTransaction() error = %v, want %v", err, rollbackError)
	}

	var count int64
	if err := resources.DB(ctx).Table(persistenceProbeTable).Count(&count).Error; err != nil {
		t.Fatalf("count probe rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("row count = %d, want 0 after rollback", count)
	}

	// 验证 PostgreSQL 唯一约束错误被 GORM 统一转换
	if err := resources.DB(ctx).
		Exec("INSERT INTO "+persistenceProbeTable+" (id, value) VALUES (?, ?)", 1, "duplicate").Error; err != nil {
		t.Fatalf("insert probe row: %v", err)
	}
	err = resources.DB(ctx).
		Exec("INSERT INTO "+persistenceProbeTable+" (id, value) VALUES (?, ?)", 2, "duplicate").Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate error = %v, want %v", err, gorm.ErrDuplicatedKey)
	}
}

// testBusinessRepositories 验证业务模块内 GORM Repository
func testBusinessRepositories(t *testing.T, ctx context.Context, resources *data.Data) {
	t.Helper()

	userRepository := usermodule.NewRepository(resources)
	createdUser, err := userRepository.Create(ctx, usermodule.CreateUser{
		Username:     "repository-user",
		Nickname:     "Repository User",
		PasswordHash: "hashed-password",
		Status:       usermodule.StatusEnabled,
	})
	if err != nil {
		t.Fatalf("user Repository.Create() error = %v", err)
	}
	credential, err := userRepository.FindCredentialByUsername(ctx, createdUser.Username)
	if err != nil || credential.User.ID != createdUser.ID || credential.PasswordHash != "hashed-password" {
		t.Fatalf("user credential = %#v, error = %v", credential, err)
	}
	_, err = userRepository.Create(ctx, usermodule.CreateUser{
		Username:     createdUser.Username,
		Nickname:     "Duplicate",
		PasswordHash: "hashed-password",
		Status:       usermodule.StatusEnabled,
	})
	assertApplicationErrorCode(t, err, usermodule.CodeUsernameExists)

	adminRepository := adminusermodule.NewRepository(resources)
	createdAdmin, err := adminRepository.Create(ctx, adminusermodule.CreateAdminUser{
		Username:     "repository-admin",
		DisplayName:  "Repository Admin",
		PasswordHash: "hashed-password",
		Status:       adminusermodule.StatusEnabled,
	})
	if err != nil {
		t.Fatalf("admin Repository.Create() error = %v", err)
	}
	if _, err := adminRepository.FindByID(ctx, createdAdmin.ID); err != nil {
		t.Fatalf("admin Repository.FindByID() error = %v", err)
	}

	configRepository := configmodule.NewRepository(resources)
	savedConfig, err := configRepository.Save(ctx, configmodule.Item{
		Key:         "repository.config",
		Value:       json.RawMessage(`{"enabled":true}`),
		Description: "Repository Config",
		Public:      true,
	})
	if err != nil {
		t.Fatalf("config Repository.Save() error = %v", err)
	}
	var configValue map[string]bool
	if err := json.Unmarshal(savedConfig.Value, &configValue); err != nil {
		t.Fatalf("decode saved config: %v", err)
	}
	if !savedConfig.Public || !configValue["enabled"] {
		t.Errorf("saved config = %#v, want public JSON config", savedConfig)
	}
}

// applyBusinessMigration 在真实测试数据库执行版本化迁移
func applyBusinessMigration(
	t *testing.T,
	ctx context.Context,
	resources *data.Data,
	filename string,
) {
	t.Helper()
	path := filepath.Join(findRepositoryRoot(t), "migrations", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", filename, err)
	}

	// 在单个事务中逐条执行迁移语句
	err = resources.WithinTransaction(ctx, func(transactionContext context.Context) error {
		for _, statement := range strings.Split(string(content), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" || statement == "BEGIN" || statement == "COMMIT" {
				continue
			}
			if err := resources.DB(transactionContext).Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("apply migration %s: %v", filename, err)
	}
}
