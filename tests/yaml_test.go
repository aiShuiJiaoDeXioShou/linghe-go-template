package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestProjectYAMLFiles 验证项目级 YAML 文件语法
func TestProjectYAMLFiles(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	paths := []string{
		"docker-compose.yaml",
		"deploy/docker-compose.stg.yaml",
		"deploy/docker-compose.production.yaml",
		".github/workflows/ci.yml",
		".github/workflows/deploy.yml",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			// 读取并解析项目级 YAML 文件
			content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(path)))
			if err != nil {
				t.Fatalf("read YAML: %v", err)
			}
			var document yaml.Node
			if err := yaml.Unmarshal(content, &document); err != nil {
				t.Fatalf("parse YAML: %v", err)
			}
		})
	}
}

// findRepositoryRoot 根据当前测试文件定位仓库根目录
func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Dir(filepath.Dir(currentFile))
}
