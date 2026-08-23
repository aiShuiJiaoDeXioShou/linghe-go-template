package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	// DefaultPath 表示默认配置文件路径
	DefaultPath = "configs/config.local.yaml"

	defaultAppName          = "go-template"
	defaultAppEnv           = "local"
	defaultHTTPHost         = "0.0.0.0"
	defaultHTTPPort         = 3000
	defaultReadTimeout      = 5 * time.Second
	defaultWriteTimeout     = 10 * time.Second
	defaultIdleTimeout      = 60 * time.Second
	defaultShutdownTimeout  = 10 * time.Second
	defaultReadinessTimeout = 2 * time.Second

	defaultPostgreSQLURL                   = "postgres://go_template:go_template_local@postgresql:5432/go_template_local?sslmode=disable"
	defaultPostgreSQLMaxOpenConnections    = 10
	defaultPostgreSQLMaxIdleConnections    = 2
	defaultPostgreSQLConnectionMaxLifetime = time.Hour
	defaultPostgreSQLConnectionMaxIdleTime = 30 * time.Minute
	defaultPostgreSQLConnectTimeout        = 5 * time.Second
	defaultPostgreSQLSlowQueryThreshold    = 200 * time.Millisecond
	defaultPostgreSQLLogLevel              = "warn"

	defaultRedisAddress      = "redis:6379"
	defaultRedisDatabase     = 0
	defaultRedisPoolSize     = 10
	defaultRedisMinIdleConns = 2
	defaultRedisDialTimeout  = 5 * time.Second
	defaultRedisReadTimeout  = 3 * time.Second
	defaultRedisWriteTimeout = 3 * time.Second

	defaultAuthSessionLifetime = 30 * 24 * time.Hour
	defaultAuthIdleTimeout     = 7 * 24 * time.Hour
	defaultAppAuthKeyPrefix    = "go-template:local:auth:app:"
	defaultAdminAuthKeyPrefix  = "go-template:local:auth:admin:"
)

// Config 表示应用的全部配置
type Config struct {
	App        App        `yaml:"app"`
	HTTP       HTTP       `yaml:"http"`
	PostgreSQL PostgreSQL `yaml:"postgresql"`
	Redis      Redis      `yaml:"redis"`
	Auth       Auth       `yaml:"auth"`
}

// App 表示应用基础配置
type App struct {
	Name string `yaml:"name"`
	Env  string `yaml:"env"`
}

// HTTP 表示 HTTP 服务配置
type HTTP struct {
	Host             string        `yaml:"host"`
	Port             int           `yaml:"port"`
	ReadTimeout      time.Duration `yaml:"read_timeout"`
	WriteTimeout     time.Duration `yaml:"write_timeout"`
	IdleTimeout      time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout  time.Duration `yaml:"shutdown_timeout"`
	ReadinessTimeout time.Duration `yaml:"readiness_timeout"`
}

// PostgreSQL 表示 PostgreSQL 连接池配置
type PostgreSQL struct {
	URL                   string        `yaml:"url"`
	MaxOpenConnections    int           `yaml:"max_open_connections"`
	MaxIdleConnections    int           `yaml:"max_idle_connections"`
	ConnectionMaxLifetime time.Duration `yaml:"connection_max_lifetime"`
	ConnectionMaxIdleTime time.Duration `yaml:"connection_max_idle_time"`
	ConnectTimeout        time.Duration `yaml:"connect_timeout"`
	SlowQueryThreshold    time.Duration `yaml:"slow_query_threshold"`
	LogLevel              string        `yaml:"log_level"`
}

// Redis 表示 Redis 客户端配置
type Redis struct {
	Address      string        `yaml:"address"`
	Username     string        `yaml:"username"`
	Password     string        `yaml:"password"`
	Database     int           `yaml:"database"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_connections"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// Auth 表示双登录域的会话配置
type Auth struct {
	SessionLifetime time.Duration `yaml:"session_lifetime"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	App             AuthRealm     `yaml:"app"`
	Admin           AuthRealm     `yaml:"admin"`
}

// AuthRealm 表示单个登录域的 Redis 键空间配置
type AuthRealm struct {
	KeyPrefix string `yaml:"key_prefix"`
}

// Address 返回适用于网络监听的主机和端口
func (c HTTP) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Load 从指定 YAML 文件读取并校验配置
func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("打开配置文件 %q: %w", path, err)
	}
	defer file.Close()

	// 创建默认配置用于合并文件内容
	cfg := defaultConfig()

	// 严格解析 YAML 并拒绝未知字段
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件 %q: %w", path, err)
	}

	// 规范化字符串配置
	cfg.App.Name = strings.TrimSpace(cfg.App.Name)
	cfg.App.Env = strings.ToLower(strings.TrimSpace(cfg.App.Env))
	cfg.HTTP.Host = strings.TrimSpace(cfg.HTTP.Host)
	cfg.PostgreSQL.URL = strings.TrimSpace(cfg.PostgreSQL.URL)
	cfg.PostgreSQL.LogLevel = strings.ToLower(strings.TrimSpace(cfg.PostgreSQL.LogLevel))
	cfg.Redis.Address = strings.TrimSpace(cfg.Redis.Address)
	cfg.Redis.Username = strings.TrimSpace(cfg.Redis.Username)
	cfg.Auth.App.KeyPrefix = normalizeKeyPrefix(cfg.Auth.App.KeyPrefix)
	cfg.Auth.Admin.KeyPrefix = normalizeKeyPrefix(cfg.Auth.Admin.KeyPrefix)

	// 校验最终配置
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("校验配置文件 %q: %w", path, err)
	}

	return cfg, nil
}

// defaultConfig 创建应用默认配置
func defaultConfig() Config {
	return Config{
		App: App{
			Name: defaultAppName,
			Env:  defaultAppEnv,
		},
		HTTP: HTTP{
			Host:             defaultHTTPHost,
			Port:             defaultHTTPPort,
			ReadTimeout:      defaultReadTimeout,
			WriteTimeout:     defaultWriteTimeout,
			IdleTimeout:      defaultIdleTimeout,
			ShutdownTimeout:  defaultShutdownTimeout,
			ReadinessTimeout: defaultReadinessTimeout,
		},
		PostgreSQL: PostgreSQL{
			URL:                   defaultPostgreSQLURL,
			MaxOpenConnections:    defaultPostgreSQLMaxOpenConnections,
			MaxIdleConnections:    defaultPostgreSQLMaxIdleConnections,
			ConnectionMaxLifetime: defaultPostgreSQLConnectionMaxLifetime,
			ConnectionMaxIdleTime: defaultPostgreSQLConnectionMaxIdleTime,
			ConnectTimeout:        defaultPostgreSQLConnectTimeout,
			SlowQueryThreshold:    defaultPostgreSQLSlowQueryThreshold,
			LogLevel:              defaultPostgreSQLLogLevel,
		},
		Redis: Redis{
			Address:      defaultRedisAddress,
			Database:     defaultRedisDatabase,
			PoolSize:     defaultRedisPoolSize,
			MinIdleConns: defaultRedisMinIdleConns,
			DialTimeout:  defaultRedisDialTimeout,
			ReadTimeout:  defaultRedisReadTimeout,
			WriteTimeout: defaultRedisWriteTimeout,
		},
		Auth: Auth{
			SessionLifetime: defaultAuthSessionLifetime,
			IdleTimeout:     defaultAuthIdleTimeout,
			App: AuthRealm{
				KeyPrefix: defaultAppAuthKeyPrefix,
			},
			Admin: AuthRealm{
				KeyPrefix: defaultAdminAuthKeyPrefix,
			},
		},
	}
}

// validate 校验应用配置
func (c Config) validate() error {
	if c.App.Name == "" {
		return fmt.Errorf("app.name 不能为空")
	}
	if c.App.Env == "" {
		return fmt.Errorf("app.env 不能为空")
	}
	if c.App.Env != "local" && c.App.Env != "stg" && c.App.Env != "production" {
		return fmt.Errorf("app.env 只允许 local stg 或 production")
	}
	if c.HTTP.Host == "" {
		return fmt.Errorf("http.host 不能为空")
	}
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("http.port 必须在 1 到 65535 之间")
	}

	timeouts := []struct {
		name  string
		value time.Duration
	}{
		{name: "http.read_timeout", value: c.HTTP.ReadTimeout},
		{name: "http.write_timeout", value: c.HTTP.WriteTimeout},
		{name: "http.idle_timeout", value: c.HTTP.IdleTimeout},
		{name: "http.shutdown_timeout", value: c.HTTP.ShutdownTimeout},
		{name: "http.readiness_timeout", value: c.HTTP.ReadinessTimeout},
		{name: "postgresql.connection_max_lifetime", value: c.PostgreSQL.ConnectionMaxLifetime},
		{name: "postgresql.connection_max_idle_time", value: c.PostgreSQL.ConnectionMaxIdleTime},
		{name: "postgresql.connect_timeout", value: c.PostgreSQL.ConnectTimeout},
		{name: "postgresql.slow_query_threshold", value: c.PostgreSQL.SlowQueryThreshold},
		{name: "redis.dial_timeout", value: c.Redis.DialTimeout},
		{name: "redis.read_timeout", value: c.Redis.ReadTimeout},
		{name: "redis.write_timeout", value: c.Redis.WriteTimeout},
		{name: "auth.session_lifetime", value: c.Auth.SessionLifetime},
		{name: "auth.idle_timeout", value: c.Auth.IdleTimeout},
	}
	for _, timeout := range timeouts {
		if timeout.value <= 0 {
			return fmt.Errorf("%s 必须大于零", timeout.name)
		}
	}

	// 校验 PostgreSQL 地址和连接池范围
	databaseURL, err := url.Parse(c.PostgreSQL.URL)
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") || databaseURL.Host == "" {
		return fmt.Errorf("postgresql.url 必须是有效的 PostgreSQL URL")
	}
	if c.PostgreSQL.MaxOpenConnections < 1 {
		return fmt.Errorf("postgresql.max_open_connections 必须大于零")
	}
	if c.PostgreSQL.MaxIdleConnections < 0 || c.PostgreSQL.MaxIdleConnections > c.PostgreSQL.MaxOpenConnections {
		return fmt.Errorf("postgresql.max_idle_connections 必须在零到 max_open_connections 之间")
	}
	if c.PostgreSQL.LogLevel != "silent" && c.PostgreSQL.LogLevel != "error" && c.PostgreSQL.LogLevel != "warn" && c.PostgreSQL.LogLevel != "info" {
		return fmt.Errorf("postgresql.log_level 只允许 silent error warn 或 info")
	}

	// 校验 Redis 地址和连接池范围
	redisHost, redisPort, err := net.SplitHostPort(c.Redis.Address)
	if err != nil || strings.TrimSpace(redisHost) == "" || strings.TrimSpace(redisPort) == "" {
		return fmt.Errorf("redis.address 必须包含有效的主机和端口")
	}
	redisPortNumber, err := strconv.Atoi(redisPort)
	if err != nil || redisPortNumber < 1 || redisPortNumber > 65535 {
		return fmt.Errorf("redis.address 端口必须在 1 到 65535 之间")
	}
	if c.Redis.Database < 0 {
		return fmt.Errorf("redis.database 不能小于零")
	}
	if c.Redis.PoolSize < 1 {
		return fmt.Errorf("redis.pool_size 必须大于零")
	}
	if c.Redis.MinIdleConns < 0 || c.Redis.MinIdleConns > c.Redis.PoolSize {
		return fmt.Errorf("redis.min_idle_connections 必须在零到 pool_size 之间")
	}

	// 校验认证会话时限和两个登录域的键空间
	if c.Auth.IdleTimeout > c.Auth.SessionLifetime {
		return fmt.Errorf("auth.idle_timeout 不能大于 auth.session_lifetime")
	}
	if err := validateAuthKeyPrefix("auth.app.key_prefix", c.Auth.App.KeyPrefix); err != nil {
		return err
	}
	if err := validateAuthKeyPrefix("auth.admin.key_prefix", c.Auth.Admin.KeyPrefix); err != nil {
		return err
	}
	if c.Auth.App.KeyPrefix == c.Auth.Admin.KeyPrefix {
		return fmt.Errorf("auth.app.key_prefix 和 auth.admin.key_prefix 必须不同")
	}

	return nil
}

func normalizeKeyPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		return prefix + ":"
	}
	return prefix
}

func validateAuthKeyPrefix(name string, prefix string) error {
	if prefix == "" {
		return fmt.Errorf("%s 不能为空", name)
	}
	if strings.ContainsAny(prefix, " \t\r\n") {
		return fmt.Errorf("%s 不能包含空白字符", name)
	}
	return nil
}
