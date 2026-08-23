package auth

import (
	"errors"
	"fmt"

	"go-template/internal/modules/identity"

	"golang.org/x/crypto/bcrypt"
)

type passwordHasher struct{}

// NewPasswordHasher 创建基于 bcrypt 的密码哈希器
func NewPasswordHasher() identity.PasswordHasher {
	return passwordHasher{}
}

// Hash 使用 bcrypt 生成自适应密码哈希
func (passwordHasher) Hash(password string) (string, error) {
	encodedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成密码哈希: %w", err)
	}
	return string(encodedPassword), nil
}

// Matches 使用 bcrypt 验证密码
func (passwordHasher) Matches(encodedPassword string, password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(encodedPassword), []byte(password))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	return false, fmt.Errorf("验证密码哈希: %w", err)
}
