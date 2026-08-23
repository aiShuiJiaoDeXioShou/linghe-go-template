package auth

import (
	"context"

	"go-template/internal/modules/identity"
)

type sessionIssuer struct {
	realm *Realm
}

// NewSessionIssuer 创建绑定指定登录域的会话签发器
func NewSessionIssuer(realm *Realm) identity.SessionIssuer {
	return &sessionIssuer{realm: realm}
}

// Issue 在绑定的登录域中创建会话
func (i *sessionIssuer) Issue(ctx context.Context, userID string, device string) (identity.Token, error) {
	token, err := i.realm.Login(ctx, userID, device)
	if err != nil {
		return identity.Token{}, err
	}
	return identity.Token{
		AccessToken: token.Value,
		TokenType:   token.Type,
		ExpiresAt:   token.ExpiresAt,
		Realm:       string(token.Realm),
	}, nil
}
