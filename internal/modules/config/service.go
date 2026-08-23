package config

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type repository interface {
	FindByKey(ctx context.Context, key string) (Item, error)
	Save(ctx context.Context, item Item) (Item, error)
}

// Service 提供系统配置的读取和保存能力
type Service struct {
	repository repository
}

// NewService 创建系统配置服务
func NewService(storage repository) *Service {
	return &Service{repository: storage}
}

// Get 返回指定系统配置
func (s *Service) Get(ctx context.Context, key string) (Item, error) {
	key = normalizeKey(key)
	if !keyPattern.MatchString(key) {
		return Item{}, NewInvalidInputError()
	}
	return s.repository.FindByKey(ctx, key)
}

// GetPublic 返回允许业务端公开读取的系统配置
func (s *Service) GetPublic(ctx context.Context, key string) (Item, error) {
	item, err := s.Get(ctx, key)
	if err != nil {
		return Item{}, err
	}
	if !item.Public {
		return Item{}, NewNotFoundError()
	}
	return item, nil
}

// Upsert 新增或更新系统配置
func (s *Service) Upsert(ctx context.Context, command UpsertCommand) (Item, error) {
	command.Key = normalizeKey(command.Key)
	command.Description = strings.TrimSpace(command.Description)
	if !keyPattern.MatchString(command.Key) ||
		!json.Valid(command.Value) ||
		utf8.RuneCountInString(command.Description) > 256 {
		return Item{}, NewInvalidInputError()
	}

	// 复制 JSON 数据避免调用方在保存期间修改内容
	return s.repository.Save(ctx, Item{
		Key:         command.Key,
		Value:       bytes.Clone(command.Value),
		Description: command.Description,
		Public:      command.Public,
	})
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
