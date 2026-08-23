// migration 包封装 PostgreSQL 版本化迁移的执行能力
package migration

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// DefaultDirectory 表示默认迁移文件目录
const DefaultDirectory = "migrations"

// State 表示数据库当前迁移版本和状态
type State struct {
	Version uint
	Dirty   bool
	Exists  bool
}

// Up 将数据库升级到最新迁移版本
func Up(ctx context.Context, databaseURL string, directory string) (err error) {
	instance, err := open(databaseURL, directory)
	if err != nil {
		return err
	}
	defer closeInstance(instance, &err)

	// 将进程取消信号转交给迁移器以便在安全点停止
	if err := execute(ctx, instance, instance.Up); err != nil {
		return fmt.Errorf("执行数据库升级: %w", err)
	}
	return nil
}

// Down 按指定步数回滚数据库迁移
func Down(ctx context.Context, databaseURL string, directory string, steps uint) (err error) {
	if steps == 0 {
		return fmt.Errorf("回滚步数必须大于零")
	}
	maximumInt := int(^uint(0) >> 1)
	if steps > uint(maximumInt) {
		return fmt.Errorf("回滚步数超过支持范围")
	}

	instance, err := open(databaseURL, directory)
	if err != nil {
		return err
	}
	defer closeInstance(instance, &err)

	// 使用负数步数只回滚明确指定的迁移数量
	if err := execute(ctx, instance, func() error {
		return instance.Steps(-int(steps))
	}); err != nil {
		return fmt.Errorf("执行数据库回滚: %w", err)
	}
	return nil
}

// Current 返回数据库当前迁移版本
func Current(databaseURL string, directory string) (state State, err error) {
	instance, err := open(databaseURL, directory)
	if err != nil {
		return State{}, err
	}
	defer closeInstance(instance, &err)

	version, dirty, err := instance.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("读取数据库迁移版本: %w", err)
	}
	return State{Version: version, Dirty: dirty, Exists: true}, nil
}

func open(databaseURL string, directory string) (*migrate.Migrate, error) {
	sourceURL, err := migrationSourceURL(directory)
	if err != nil {
		return nil, err
	}
	driverURL, err := pgxDatabaseURL(databaseURL)
	if err != nil {
		return nil, err
	}

	// 使用 pgx v5 驱动连接目标数据库并创建版本记录表
	instance, err := migrate.New(sourceURL, driverURL)
	if err != nil {
		return nil, fmt.Errorf("创建数据库迁移器: %w", err)
	}
	return instance, nil
}

func migrationSourceURL(directory string) (string, error) {
	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("解析迁移目录 %q: %w", directory, err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(absoluteDirectory)}).String(), nil
}

func pgxDatabaseURL(databaseURL string) (string, error) {
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("解析 PostgreSQL URL: %w", err)
	}
	if parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql" {
		return "", fmt.Errorf("数据库迁移只支持 PostgreSQL URL")
	}
	parsedURL.Scheme = "pgx5"
	return parsedURL.String(), nil
}

func execute(ctx context.Context, instance *migrate.Migrate, operation func() error) error {
	completed := make(chan struct{})
	defer close(completed)

	go func() {
		select {
		case <-ctx.Done():
			select {
			case instance.GracefulStop <- true:
			case <-completed:
			}
		case <-completed:
		}
	}()

	err := operation()
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	if err != nil {
		return err
	}
	return ctx.Err()
}

func closeInstance(instance *migrate.Migrate, target *error) {
	sourceErr, databaseErr := instance.Close()
	if sourceErr != nil {
		sourceErr = fmt.Errorf("关闭迁移文件源: %w", sourceErr)
	}
	if databaseErr != nil {
		databaseErr = fmt.Errorf("关闭迁移数据库连接: %w", databaseErr)
	}
	*target = errors.Join(*target, sourceErr, databaseErr)
}
