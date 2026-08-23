package user

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"go-template/internal/modules/identity"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

type repository interface {
	Create(ctx context.Context, input CreateUser) (User, error)
	FindCredentialByUsername(ctx context.Context, username string) (Credential, error)
	FindByID(ctx context.Context, id string) (User, error)
}

// Service 提供业务用户的注册 登录和资料查询能力
type Service struct {
	repository repository
	passwords  identity.PasswordHasher
	sessions   identity.SessionIssuer
}

// NewService 创建业务用户服务
func NewService(
	storage repository,
	passwords identity.PasswordHasher,
	sessions identity.SessionIssuer,
) *Service {
	return &Service{
		repository: storage,
		passwords:  passwords,
		sessions:   sessions,
	}
}

// Register 注册业务用户并保存密码哈希
func (s *Service) Register(ctx context.Context, command RegisterCommand) (User, error) {
	command.Username = normalizeUsername(command.Username)
	command.Nickname = strings.TrimSpace(command.Nickname)
	if command.Nickname == "" {
		command.Nickname = command.Username
	}
	if !validUsername(command.Username) ||
		!validPassword(command.Password) ||
		utf8.RuneCountInString(command.Nickname) > 64 {
		return User{}, NewInvalidInputError()
	}

	// 在进入持久层前生成不可逆密码哈希
	passwordHash, err := s.passwords.Hash(command.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash user password: %w", err)
	}
	createdUser, err := s.repository.Create(ctx, CreateUser{
		Username:     command.Username,
		Nickname:     command.Nickname,
		PasswordHash: passwordHash,
		Status:       StatusEnabled,
	})
	if err != nil {
		return User{}, err
	}
	return createdUser, nil
}

// Login 校验业务用户凭据并签发 App 会话
func (s *Service) Login(ctx context.Context, command LoginCommand) (identity.Token, error) {
	command.Username = normalizeUsername(command.Username)
	command.Device = strings.TrimSpace(command.Device)
	if command.Username == "" || command.Password == "" || command.Device == "" || len(command.Device) > 64 {
		return identity.Token{}, NewInvalidInputError()
	}

	// 使用统一错误隐藏用户名是否存在
	credential, err := s.repository.FindCredentialByUsername(ctx, command.Username)
	if err != nil {
		if IsNotFound(err) {
			return identity.Token{}, NewInvalidCredentialsError()
		}
		return identity.Token{}, err
	}
	matched, err := s.passwords.Matches(credential.PasswordHash, command.Password)
	if err != nil {
		return identity.Token{}, fmt.Errorf("verify user password: %w", err)
	}
	if !matched {
		return identity.Token{}, NewInvalidCredentialsError()
	}
	if credential.User.Status != StatusEnabled {
		return identity.Token{}, NewDisabledError()
	}

	// 凭据和状态通过后签发 App 登录域会话
	return s.sessions.Issue(ctx, credential.User.ID, command.Device)
}

// GetProfile 返回指定业务用户的公开资料
func (s *Service) GetProfile(ctx context.Context, userID string) (User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return User{}, NewInvalidInputError()
	}
	return s.repository.FindByID(ctx, userID)
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validUsername(username string) bool {
	return usernamePattern.MatchString(username)
}

func validPassword(password string) bool {
	length := len([]byte(password))
	return length >= 8 && length <= 72
}
