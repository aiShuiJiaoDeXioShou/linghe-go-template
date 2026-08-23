package config

import (
	"encoding/json"
	"time"
)

// Item 表示一个系统配置项
type Item struct {
	Key         string
	Value       json.RawMessage
	Description string
	Public      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UpsertCommand 表示新增或更新系统配置的命令
type UpsertCommand struct {
	Key         string
	Value       json.RawMessage
	Description string
	Public      bool
}
