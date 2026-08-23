package models

import "time"

// AdminUser 映射管理员用户表
type AdminUser struct {
	ID           string    `gorm:"column:id;type:uuid;primaryKey"`
	Username     string    `gorm:"column:username;size:32;not null;uniqueIndex:admin_users_username_key"`
	PasswordHash string    `gorm:"column:password_hash;size:255;not null"`
	DisplayName  string    `gorm:"column:display_name;size:64;not null"`
	Status       string    `gorm:"column:status;size:16;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

// TableName 返回管理员用户表名
func (AdminUser) TableName() string {
	return "admin_users"
}
