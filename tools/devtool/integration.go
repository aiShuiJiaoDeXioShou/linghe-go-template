package devtool

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go-template/internal/config"
)

var databaseIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (c command) testIntegration(ctx context.Context) error {
	cfg, err := config.Load(filepath.Join(c.root, "configs", "config.local.yaml"))
	if err != nil {
		return err
	}
	databaseURL, err := url.Parse(cfg.PostgreSQL.URL)
	if err != nil {
		return fmt.Errorf("解析本地 PostgreSQL URL: %w", err)
	}
	databaseName := strings.ReplaceAll(cfg.App.Name, "-", "_") + "_test"
	if !databaseIdentifierPattern.MatchString(databaseName) || !strings.HasSuffix(databaseName, "_test") {
		return fmt.Errorf("测试数据库名称不安全: %q", databaseName)
	}
	username := databaseURL.User.Username()
	if username == "" {
		return fmt.Errorf("本地 PostgreSQL URL 缺少用户名")
	}
	postgresPort, err := availableLocalPort()
	if err != nil {
		return err
	}
	redisPort, err := availableLocalPort()
	if err != nil {
		return err
	}
	for redisPort == postgresPort {
		redisPort, err = availableLocalPort()
		if err != nil {
			return err
		}
	}
	databaseURL.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(postgresPort))
	databaseURL.Path = "/" + databaseName
	redisAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(redisPort))
	composeEnvironment := []string{
		"POSTGRES_PORT=" + strconv.Itoa(postgresPort),
		"REDIS_PORT=" + strconv.Itoa(redisPort),
	}

	_, _ = fmt.Fprintf(c.stdout, "> 使用临时端口 PostgreSQL=%d Redis=%d\n", postgresPort, redisPort)
	if err := c.runProcess(ctx, composeEnvironment, "docker", "compose", "up", "-d", "--wait", "postgresql", "redis"); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.stdout, "> 重建隔离测试数据库 %s\n", databaseName)
	if err := c.runProcess(ctx, nil, "docker", "compose", "exec", "-T", "postgresql", "dropdb", "--if-exists", "--force", "-U", username, databaseName); err != nil {
		return err
	}
	if err := c.runProcess(ctx, nil, "docker", "compose", "exec", "-T", "postgresql", "createdb", "-U", username, databaseName); err != nil {
		return err
	}
	environment := []string{
		"TEST_POSTGRES_URL=" + databaseURL.String(),
		"TEST_REDIS_ADDRESS=" + redisAddress,
		"TEST_REDIS_PASSWORD=" + cfg.Redis.Password,
	}
	return c.runProcess(ctx, environment, "go", "test", "./...")
}

func availableLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("分配本地测试端口: %w", err)
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.Port == 0 {
		return 0, fmt.Errorf("读取本地测试端口失败")
	}
	return address.Port, nil
}
