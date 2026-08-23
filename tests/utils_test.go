package tests

import (
	"testing"

	"go-template/internal/utils"

	"github.com/google/uuid"
)

// TestNewUUID 验证 UUID 生成结果有效且连续调用不重复
func TestNewUUID(t *testing.T) {
	first := utils.NewUUID()
	second := utils.NewUUID()

	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("NewUUID() = %q, want valid UUID: %v", first, err)
	}
	if first == second {
		t.Fatalf("NewUUID() returned duplicate value %q", first)
	}
}
