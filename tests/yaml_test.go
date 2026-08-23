package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestProjectYAMLFiles 验证仓库中现有 YAML 文件语法
func TestProjectYAMLFiles(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	var paths []string
	if err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != repositoryRoot && (entry.Name() == ".git" || entry.Name() == ".deploy" || entry.Name() == "dist" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		extension := filepath.Ext(path)
		if extension == ".yaml" || extension == ".yml" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("scan YAML files: %v", err)
	}

	for _, path := range paths {
		relativePath, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			t.Fatalf("relative YAML path: %v", err)
		}
		t.Run(filepath.ToSlash(relativePath), func(t *testing.T) {
			// 读取并解析项目级 YAML 文件
			content, err := os.ReadFile(path)
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
