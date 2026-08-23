package models

import "time"

// User 映射业务用户表
type User struct {
	ID           string    `gorm:"column:id;type:uuid;primaryKey"`
	Username     string    `gorm:"column:username;size:32;not null;uniqueIndex:users_username_key"`
	PasswordHash string    `gorm:"column:password_hash;size:255;not null"`
	Nickname     string    `gorm:"column:nickname;size:64;not null"`
	Status       string    `gorm:"column:status;size:16;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

// TableName 返回业务用户表名
func (User) TableName() string {
	return "users"
}
