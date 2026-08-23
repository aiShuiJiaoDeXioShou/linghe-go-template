package identity

import (
	"context"
	"time"
)

// Token 表示业务层可使用的不透明登录令牌
type Token struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	Realm       string    `json:"realm"`
}

// PasswordHasher 定义业务层需要的密码哈希能力
type PasswordHasher interface {
	Hash(password string) (string, error)
	Matches(encodedPassword string, password string) (bool, error)
}

// SessionIssuer 定义业务层需要的会话签发能力
type SessionIssuer interface {
	Issue(ctx context.Context, userID string, device string) (Token, error)
}
