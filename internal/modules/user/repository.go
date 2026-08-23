package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go-template/internal/data"
	"go-template/internal/models"
	"go-template/internal/utils"

	"gorm.io/gorm"
)

// Repository 使用 GORM 实现业务用户持久化
type Repository struct {
	data *data.Data
}

// NewRepository 创建业务用户 GORM Repository
func NewRepository(resources *data.Data) *Repository {
	return &Repository{data: resources}
}

// Create 创建业务用户并转换唯一约束错误
func (r *Repository) Create(ctx context.Context, input CreateUser) (User, error) {
	now := time.Now().UTC()
	record := models.User{
		ID:           utils.NewUUID(),
		Username:     input.Username,
		PasswordHash: input.PasswordHash,
		Nickname:     input.Nickname,
		Status:       string(input.Status),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := r.data.DB(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return User{}, NewUsernameExistsError()
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return toUser(record), nil
}

// FindCredentialByUsername 查询登录所需的业务用户凭据
func (r *Repository) FindCredentialByUsername(ctx context.Context, username string) (Credential, error) {
	var record models.User
	err := r.data.DB(ctx).
		Select("id", "username", "password_hash", "nickname", "status", "created_at", "updated_at").
		Where("username = ?", username).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Credential{}, NewNotFoundError()
	}
	if err != nil {
		return Credential{}, fmt.Errorf("find user credential: %w", err)
	}
	return Credential{User: toUser(record), PasswordHash: record.PasswordHash}, nil
}

// FindByID 查询业务用户公开资料
func (r *Repository) FindByID(ctx context.Context, id string) (User, error) {
	var record models.User
	err := r.data.DB(ctx).
		Select("id", "username", "nickname", "status", "created_at", "updated_at").
		Where("id = ?", id).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, NewNotFoundError()
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return toUser(record), nil
}

// toUser 将数据库模型转换为业务实体
func toUser(record models.User) User {
	return User{
		ID:        record.ID,
		Username:  record.Username,
		Nickname:  record.Nickname,
		Status:    Status(record.Status),
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}
