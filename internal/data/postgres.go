package data

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"go-template/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgres 创建 GORM 客户端和底层连接池
func openPostgres(ctx context.Context, cfg config.PostgreSQL) (*gorm.DB, *sql.DB, error) {
	// 创建 GORM 日志组件和运行配置
	gormLogger := logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
		SlowThreshold:             cfg.SlowQueryThreshold,
		LogLevel:                  parseLogLevel(cfg.LogLevel),
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	})
	database, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{
		Logger:               gormLogger,
		TranslateError:       true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("创建 GORM PostgreSQL 客户端: %w", err)
	}

	// 获取并配置 database/sql 连接池
	pool, err := database.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("获取 PostgreSQL 连接池: %w", err)
	}
	pool.SetMaxOpenConns(cfg.MaxOpenConnections)
	pool.SetMaxIdleConns(cfg.MaxIdleConnections)
	pool.SetConnMaxLifetime(cfg.ConnectionMaxLifetime)
	pool.SetConnMaxIdleTime(cfg.ConnectionMaxIdleTime)

	// 在启动阶段确认 PostgreSQL 可用
	pingContext, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.PingContext(pingContext); err != nil {
		_ = pool.Close()
		return nil, nil, fmt.Errorf("连接 PostgreSQL: %w", err)
	}

	return database, pool, nil
}

// parseLogLevel 转换 GORM 日志级别
func parseLogLevel(level string) logger.LogLevel {
	switch level {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "info":
		return logger.Info
	default:
		return logger.Warn
	}
}
