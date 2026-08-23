package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go-template/internal/data"
	"go-template/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 使用 GORM 实现系统配置持久化
type Repository struct {
	data *data.Data
}

// NewRepository 创建系统配置 GORM Repository
func NewRepository(resources *data.Data) *Repository {
	return &Repository{data: resources}
}

// FindByKey 查询指定系统配置
func (r *Repository) FindByKey(ctx context.Context, key string) (Item, error) {
	var record models.SystemConfig
	err := r.data.DB(ctx).
		Select("config_key", "config_value", "description", "is_public", "created_at", "updated_at").
		Where("config_key = ?", key).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Item{}, NewNotFoundError()
	}
	if err != nil {
		return Item{}, fmt.Errorf("find system config: %w", err)
	}
	return toItem(record), nil
}

// Save 使用配置键新增或更新系统配置
func (r *Repository) Save(ctx context.Context, item Item) (Item, error) {
	now := time.Now().UTC()
	record := models.SystemConfig{
		Key:         item.Key,
		Value:       string(item.Value),
		Description: item.Description,
		Public:      item.Public,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 使用 PostgreSQL 冲突更新保证单个配置键只有一条记录
	err := r.data.DB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "config_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"config_value": string(item.Value),
			"description":  item.Description,
			"is_public":    item.Public,
			"updated_at":   now,
		}),
	}).Create(&record).Error
	if err != nil {
		return Item{}, fmt.Errorf("save system config: %w", err)
	}
	return r.FindByKey(ctx, item.Key)
}

// toItem 将数据库模型转换为业务实体
func toItem(record models.SystemConfig) Item {
	return Item{
		Key:         record.Key,
		Value:       json.RawMessage(record.Value),
		Description: record.Description,
		Public:      record.Public,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}
