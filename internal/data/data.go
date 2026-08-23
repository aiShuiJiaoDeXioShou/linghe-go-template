package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go-template/internal/config"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Data 聚合数据库和缓存资源并管理它们的生命周期
type Data struct {
	database    *gorm.DB
	pool        *sql.DB
	redisClient *redis.Client
}

// Open 创建并验证 PostgreSQL 和 Redis 资源
func Open(ctx context.Context, cfg config.Config) (*Data, error) {
	// 先建立 PostgreSQL 连接并完成连接池配置
	database, pool, err := openPostgres(ctx, cfg.PostgreSQL)
	if err != nil {
		return nil, err
	}

	// PostgreSQL 成功后再创建 Redis 客户端
	redisClient, err := openRedis(ctx, cfg.Redis)
	if err != nil {
		_ = pool.Close()
		return nil, err
	}

	return &Data{
		database:    database,
		pool:        pool,
		redisClient: redisClient,
	}, nil
}

// DB 返回绑定上下文和当前事务的 GORM 会话
func (d *Data) DB(ctx context.Context) *gorm.DB {
	if transaction, ok := transactionFromContext(ctx); ok {
		return transaction.WithContext(ctx)
	}
	return d.database.WithContext(ctx)
}

// Redis 返回共享的 Redis 客户端
func (d *Data) Redis() *redis.Client {
	return d.redisClient
}

// Ping 检查所有数据资源是否可用
func (d *Data) Ping(ctx context.Context) error {
	// 先验证 PostgreSQL 再检查 Redis
	if err := d.PingDatabase(ctx); err != nil {
		return err
	}
	if err := d.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}

// PingDatabase 执行轻量查询验证 PostgreSQL 连接
func (d *Data) PingDatabase(ctx context.Context) error {
	// 探针直接使用基础 GORM 会话避免参与业务事务
	if err := d.database.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}

// SQLStats 返回 PostgreSQL 连接池统计信息
func (d *Data) SQLStats() sql.DBStats {
	return d.pool.Stats()
}

// Close 关闭所有数据资源
func (d *Data) Close() error {
	var closeErrors []error
	if err := d.redisClient.Close(); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("close Redis: %w", err))
	}
	if err := d.pool.Close(); err != nil {
		closeErrors = append(closeErrors, fmt.Errorf("close PostgreSQL: %w", err))
	}
	return errors.Join(closeErrors...)
}
