package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	adminusermodule "go-template/internal/modules/adminuser"
	configmodule "go-template/internal/modules/config"
	"go-template/internal/modules/identity"
	usermodule "go-template/internal/modules/user"
)

// TestUserServiceRegisterAndLogin 验证业务用户注册和登录流程
func TestUserServiceRegisterAndLogin(t *testing.T) {
	repository := newFakeUserRepository()
	sessions := &fakeSessionIssuer{}
	service := usermodule.NewService(repository, fakePasswordHasher{}, sessions)

	createdUser, err := service.Register(context.Background(), usermodule.RegisterCommand{
		Username: " Alice ",
		Password: "password-123",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if createdUser.Username != "alice" || createdUser.Nickname != "alice" {
		t.Errorf("created user = %#v, want normalized username and nickname", createdUser)
	}
	if repository.credential.PasswordHash != "hashed:password-123" {
		t.Errorf("password hash = %q, want hashed password", repository.credential.PasswordHash)
	}

	token, err := service.Login(context.Background(), usermodule.LoginCommand{
		Username: "ALICE",
		Password: "password-123",
		Device:   "ios",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token.AccessToken != "session-token" || sessions.userID != createdUser.ID || sessions.device != "ios" {
		t.Errorf("login token = %#v, session user = %q device = %q", token, sessions.userID, sessions.device)
	}

	_, err = service.Login(context.Background(), usermodule.LoginCommand{
		Username: "alice",
		Password: "wrong-password",
		Device:   "ios",
	})
	assertApplicationErrorCode(t, err, usermodule.CodeInvalidCredentials)
}

// TestUserServiceRejectsDisabledUser 验证禁用业务用户不能登录
func TestUserServiceRejectsDisabledUser(t *testing.T) {
	repository := newFakeUserRepository()
	repository.credential = usermodule.Credential{
		User: usermodule.User{
			ID:       "user-disabled",
			Username: "disabled",
			Status:   usermodule.StatusDisabled,
		},
		PasswordHash: "hashed:password-123",
	}
	service := usermodule.NewService(repository, fakePasswordHasher{}, &fakeSessionIssuer{})

	_, err := service.Login(context.Background(), usermodule.LoginCommand{
		Username: "disabled",
		Password: "password-123",
		Device:   "web",
	})
	assertApplicationErrorCode(t, err, usermodule.CodeDisabled)
}

// TestAdminUserServiceCreateAndLogin 验证管理员用户创建和登录流程
func TestAdminUserServiceCreateAndLogin(t *testing.T) {
	repository := newFakeAdminUserRepository()
	sessions := &fakeSessionIssuer{}
	service := adminusermodule.NewService(repository, fakePasswordHasher{}, sessions)

	createdUser, err := service.Create(context.Background(), adminusermodule.CreateCommand{
		Username: " Root ",
		Password: "password-456",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if createdUser.Username != "root" || createdUser.DisplayName != "root" {
		t.Errorf("created admin user = %#v, want normalized fields", createdUser)
	}

	token, err := service.Login(context.Background(), adminusermodule.LoginCommand{
		Username: "root",
		Password: "password-456",
		Device:   "web",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token.Realm != "test" || sessions.userID != createdUser.ID {
		t.Errorf("login token = %#v, session user = %q", token, sessions.userID)
	}
}

// TestConfigServiceControlsPublicVisibility 验证系统配置公开读取规则
func TestConfigServiceControlsPublicVisibility(t *testing.T) {
	repository := &fakeConfigRepository{items: make(map[string]configmodule.Item)}
	service := configmodule.NewService(repository)

	publicItem, err := service.Upsert(context.Background(), configmodule.UpsertCommand{
		Key:         " App.Banner ",
		Value:       json.RawMessage(`{"enabled":true}`),
		Description: "首页横幅",
		Public:      true,
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if publicItem.Key != "app.banner" {
		t.Errorf("config key = %q, want %q", publicItem.Key, "app.banner")
	}
	if _, err := service.GetPublic(context.Background(), "app.banner"); err != nil {
		t.Errorf("GetPublic() error = %v", err)
	}

	_, err = service.Upsert(context.Background(), configmodule.UpsertCommand{
		Key:    "admin.secret",
		Value:  json.RawMessage(`{"enabled":false}`),
		Public: false,
	})
	if err != nil {
		t.Fatalf("Upsert() private error = %v", err)
	}
	_, err = service.GetPublic(context.Background(), "admin.secret")
	assertApplicationErrorCode(t, err, configmodule.CodeNotFound)

	_, err = service.Upsert(context.Background(), configmodule.UpsertCommand{
		Key:   "invalid",
		Value: json.RawMessage(`{"broken"`),
	})
	assertApplicationErrorCode(t, err, configmodule.CodeInvalidInput)
}

type fakePasswordHasher struct{}

// Hash 返回可验证的测试密码哈希
func (fakePasswordHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}

// Matches 验证测试密码哈希
func (fakePasswordHasher) Matches(encodedPassword string, password string) (bool, error) {
	return encodedPassword == "hashed:"+password, nil
}

type fakeSessionIssuer struct {
	userID string
	device string
}

// Issue 记录测试会话主体并返回固定令牌
func (i *fakeSessionIssuer) Issue(_ context.Context, userID string, device string) (identity.Token, error) {
	i.userID = userID
	i.device = device
	return identity.Token{
		AccessToken: "session-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
		Realm:       "test",
	}, nil
}

type fakeUserRepository struct {
	credential usermodule.Credential
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{}
}

// Create 保存测试业务用户
func (r *fakeUserRepository) Create(_ context.Context, input usermodule.CreateUser) (usermodule.User, error) {
	createdUser := usermodule.User{
		ID:        "user-1",
		Username:  input.Username,
		Nickname:  input.Nickname,
		Status:    input.Status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	r.credential = usermodule.Credential{User: createdUser, PasswordHash: input.PasswordHash}
	return createdUser, nil
}

// FindCredentialByUsername 查询测试业务用户凭据
func (r *fakeUserRepository) FindCredentialByUsername(_ context.Context, username string) (usermodule.Credential, error) {
	if r.credential.User.Username != username {
		return usermodule.Credential{}, usermodule.NewNotFoundError()
	}
	return r.credential, nil
}

// FindByID 查询测试业务用户
func (r *fakeUserRepository) FindByID(_ context.Context, id string) (usermodule.User, error) {
	if r.credential.User.ID != id {
		return usermodule.User{}, usermodule.NewNotFoundError()
	}
	return r.credential.User, nil
}

type fakeAdminUserRepository struct {
	credential adminusermodule.Credential
}

func newFakeAdminUserRepository() *fakeAdminUserRepository {
	return &fakeAdminUserRepository{}
}

// Create 保存测试管理员用户
func (r *fakeAdminUserRepository) Create(_ context.Context, input adminusermodule.CreateAdminUser) (adminusermodule.AdminUser, error) {
	createdUser := adminusermodule.AdminUser{
		ID:          "admin-1",
		Username:    input.Username,
		DisplayName: input.DisplayName,
		Status:      input.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	r.credential = adminusermodule.Credential{AdminUser: createdUser, PasswordHash: input.PasswordHash}
	return createdUser, nil
}

// FindCredentialByUsername 查询测试管理员凭据
func (r *fakeAdminUserRepository) FindCredentialByUsername(_ context.Context, username string) (adminusermodule.Credential, error) {
	if r.credential.AdminUser.Username != username {
		return adminusermodule.Credential{}, adminusermodule.NewNotFoundError()
	}
	return r.credential, nil
}

// FindByID 查询测试管理员用户
func (r *fakeAdminUserRepository) FindByID(_ context.Context, id string) (adminusermodule.AdminUser, error) {
	if r.credential.AdminUser.ID != id {
		return adminusermodule.AdminUser{}, adminusermodule.NewNotFoundError()
	}
	return r.credential.AdminUser, nil
}

type fakeConfigRepository struct {
	items map[string]configmodule.Item
}

// FindByKey 查询测试系统配置
func (r *fakeConfigRepository) FindByKey(_ context.Context, key string) (configmodule.Item, error) {
	item, ok := r.items[key]
	if !ok {
		return configmodule.Item{}, configmodule.NewNotFoundError()
	}
	return item, nil
}

// Save 保存测试系统配置
func (r *fakeConfigRepository) Save(_ context.Context, item configmodule.Item) (configmodule.Item, error) {
	if item.Key == "" {
		return configmodule.Item{}, fmt.Errorf("empty config key")
	}
	now := time.Now()
	if previous, ok := r.items[item.Key]; ok {
		item.CreatedAt = previous.CreatedAt
	} else {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	item.Value = bytes.Clone(item.Value)
	r.items[item.Key] = item
	return item, nil
}
