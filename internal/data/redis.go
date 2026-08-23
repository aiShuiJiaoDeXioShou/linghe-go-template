package data

import (
	"context"
	"fmt"

	"go-template/internal/config"

	"github.com/redis/go-redis/v9"
)

// openRedis 创建 Redis 客户端并验证连接
func openRedis(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	// 使用 YAML 配置创建 Redis 连接池
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.Database,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// 在启动阶段确认 Redis 可用
	pingContext, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()
	if err := client.Ping(pingContext).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("连接 Redis: %w", err)
	}

	return client, nil
}
