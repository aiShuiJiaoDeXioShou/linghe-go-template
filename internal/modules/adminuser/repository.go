package adminuser

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

// Repository 使用 GORM 实现管理员用户持久化
type Repository struct {
	data *data.Data
}

// NewRepository 创建管理员用户 GORM Repository
func NewRepository(resources *data.Data) *Repository {
	return &Repository{data: resources}
}

// Create 创建管理员用户并转换唯一约束错误
func (r *Repository) Create(ctx context.Context, input CreateAdminUser) (AdminUser, error) {
	now := time.Now().UTC()
	record := models.AdminUser{
		ID:           utils.NewUUID(),
		Username:     input.Username,
		PasswordHash: input.PasswordHash,
		DisplayName:  input.DisplayName,
		Status:       string(input.Status),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := r.data.DB(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return AdminUser{}, NewUsernameExistsError()
		}
		return AdminUser{}, fmt.Errorf("create admin user: %w", err)
	}
	return toAdminUser(record), nil
}

// FindCredentialByUsername 查询登录所需的管理员凭据
func (r *Repository) FindCredentialByUsername(ctx context.Context, username string) (Credential, error) {
	var record models.AdminUser
	err := r.data.DB(ctx).
		Select("id", "username", "password_hash", "display_name", "status", "created_at", "updated_at").
		Where("username = ?", username).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Credential{}, NewNotFoundError()
	}
	if err != nil {
		return Credential{}, fmt.Errorf("find admin user credential: %w", err)
	}
	return Credential{AdminUser: toAdminUser(record), PasswordHash: record.PasswordHash}, nil
}

// FindByID 查询管理员用户公开资料
func (r *Repository) FindByID(ctx context.Context, id string) (AdminUser, error) {
	var record models.AdminUser
	err := r.data.DB(ctx).
		Select("id", "username", "display_name", "status", "created_at", "updated_at").
		Where("id = ?", id).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminUser{}, NewNotFoundError()
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("find admin user: %w", err)
	}
	return toAdminUser(record), nil
}

// toAdminUser 将数据库模型转换为业务实体
func toAdminUser(record models.AdminUser) AdminUser {
	return AdminUser{
		ID:          record.ID,
		Username:    record.Username,
		DisplayName: record.DisplayName,
		Status:      Status(record.Status),
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}
