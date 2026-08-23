package models

import "time"

// SystemConfig 映射系统配置表
type SystemConfig struct {
	Key         string    `gorm:"column:config_key;size:128;primaryKey"`
	Value       string    `gorm:"column:config_value;type:jsonb;not null"`
	Description string    `gorm:"column:description;size:256;not null"`
	Public      bool      `gorm:"column:is_public;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

// TableName 返回系统配置表名
func (SystemConfig) TableName() string {
	return "system_configs"
}
