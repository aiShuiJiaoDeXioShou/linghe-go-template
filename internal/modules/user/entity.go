package user

import "time"

// Status 表示业务用户状态
type Status string

const (
	// StatusEnabled 表示业务用户可以正常使用
	StatusEnabled Status = "enabled"
	// StatusDisabled 表示业务用户已被禁用
	StatusDisabled Status = "disabled"
)

// User 表示不包含认证凭据的业务用户
type User struct {
	ID        string
	Username  string
	Nickname  string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Credential 表示仅在登录流程内部使用的用户凭据
type Credential struct {
	User         User
	PasswordHash string
}

// CreateUser 表示持久层创建业务用户所需的数据
type CreateUser struct {
	Username     string
	Nickname     string
	PasswordHash string
	Status       Status
}

// RegisterCommand 表示注册业务用户的命令
type RegisterCommand struct {
	Username string
	Password string
	Nickname string
}

// LoginCommand 表示业务用户登录命令
type LoginCommand struct {
	Username string
	Password string
	Device   string
}
