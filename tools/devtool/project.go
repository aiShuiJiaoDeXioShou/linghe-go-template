package devtool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go-template/internal/config"

	"golang.org/x/mod/module"
)

var projectNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type projectOptions struct {
	modulePath string
	name       string
	dryRun     bool
}

type projectChange struct {
	path     string
	original []byte
	updated  []byte
}

func (c command) initProject(ctx context.Context, args []string) error {
	options, err := parseProjectOptions(args)
	if err != nil {
		return err
	}
	oldModule, err := modulePath(c.root)
	if err != nil {
		return err
	}
	localConfig, err := config.Load(filepath.Join(c.root, "configs", "config.local.yaml"))
	if err != nil {
		return err
	}
	oldName := localConfig.App.Name
	if oldModule == options.modulePath && oldName == options.name {
		return fmt.Errorf("项目模块和名称均未发生变化")
	}

	changes, err := collectProjectChanges(c.root, oldModule, options.modulePath, oldName, options.name)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return fmt.Errorf("没有找到需要初始化的项目标识")
	}
	if options.dryRun {
		_, _ = fmt.Fprintf(c.stdout, "module: %s -> %s\nname: %s -> %s\n", oldModule, options.modulePath, oldName, options.name)
		for _, change := range changes {
			_, _ = fmt.Fprintf(c.stdout, "update %s\n", relative(c.root, change.path))
		}
		return nil
	}
	if err := applyProjectChanges(changes); err != nil {
		return err
	}
	for _, change := range changes {
		_, _ = fmt.Fprintf(c.stdout, "updated %s\n", relative(c.root, change.path))
	}
	if err := c.runProcess(ctx, nil, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("项目标识已更新 依赖整理失败: %w", err)
	}
	if err := c.runProcess(ctx, nil, "go", "fmt", "./..."); err != nil {
		return fmt.Errorf("项目标识已更新 格式化失败: %w", err)
	}
	if apiDocsConfigured(c.root) {
		if err := c.generateAPIDocs(ctx); err != nil {
			return fmt.Errorf("项目标识已更新 API 文档生成失败: %w", err)
		}
	}
	if err := c.check(ctx); err != nil {
		return fmt.Errorf("项目标识已更新 质量检查失败: %w", err)
	}
	_, _ = fmt.Fprintln(c.stdout, "项目初始化完成 请检查数据库凭据和 Git 远端地址")
	return nil
}

func parseProjectOptions(args []string) (projectOptions, error) {
	options := projectOptions{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--module":
			index++
			if index >= len(args) {
				return projectOptions{}, fmt.Errorf("--module 缺少值")
			}
			options.modulePath = strings.TrimSpace(args[index])
		case "--name":
			index++
			if index >= len(args) {
				return projectOptions{}, fmt.Errorf("--name 缺少值")
			}
			options.name = strings.TrimSpace(args[index])
		case "--dry-run":
			options.dryRun = true
		default:
			return projectOptions{}, fmt.Errorf("未知参数 %q", args[index])
		}
	}
	if err := module.CheckPath(options.modulePath); err != nil {
		return projectOptions{}, fmt.Errorf("模块路径无效: %w", err)
	}
	if !projectNamePattern.MatchString(options.name) {
		return projectOptions{}, fmt.Errorf("项目名称必须匹配 %s", projectNamePattern.String())
	}
	return options, nil
}

func collectProjectChanges(
	root string,
	oldModule string,
	newModule string,
	oldName string,
	newName string,
) ([]projectChange, error) {
	changes := make([]projectChange, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipProjectDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isProjectTextFile(path) {
			return nil
		}
		original, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := replaceProjectContent(path, original, oldModule, newModule, oldName, newName)
		if !bytes.Equal(original, updated) {
			changes = append(changes, projectChange{path: path, original: original, updated: updated})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描项目文件: %w", err)
	}
	sort.Slice(changes, func(left int, right int) bool {
		return changes[left].path < changes[right].path
	})
	return changes, nil
}

func replaceProjectContent(
	path string,
	content []byte,
	oldModule string,
	newModule string,
	oldName string,
	newName string,
) []byte {
	const modulePlaceholder = "__PROJECT_MODULE_PATH_PLACEHOLDER__"
	updated := append([]byte(nil), content...)
	if oldModule == oldName {
		if filepath.Ext(path) == ".go" {
			updated = bytes.ReplaceAll(updated, []byte(`"`+oldModule+`/`), []byte(`"`+modulePlaceholder+`/`))
			updated = bytes.ReplaceAll(updated, []byte(`"`+oldModule+`"`), []byte(`"`+modulePlaceholder+`"`))
		}
		if filepath.Base(path) == "go.mod" {
			updated = bytes.ReplaceAll(updated, []byte("module "+oldModule), []byte("module "+modulePlaceholder))
		}
	} else {
		updated = bytes.ReplaceAll(updated, []byte(oldModule), []byte(modulePlaceholder))
	}
	oldDatabaseName := strings.ReplaceAll(oldName, "-", "_")
	newDatabaseName := strings.ReplaceAll(newName, "-", "_")
	updated = bytes.ReplaceAll(updated, []byte(oldDatabaseName), []byte(newDatabaseName))
	updated = bytes.ReplaceAll(updated, []byte(oldName), []byte(newName))
	updated = bytes.ReplaceAll(updated, []byte(modulePlaceholder), []byte(newModule))
	return updated
}

func shouldSkipProjectDirectory(name string) bool {
	return name == ".git" || name == ".deploy" || name == "dist" || name == "vendor"
}

func isProjectTextFile(path string) bool {
	base := filepath.Base(path)
	if base == "Dockerfile" || base == ".gitmessage" {
		return true
	}
	switch filepath.Ext(path) {
	case ".go", ".mod", ".md", ".yaml", ".yml", ".sh", ".sql":
		return true
	default:
		return false
	}
}

func applyProjectChanges(changes []projectChange) error {
	updated := make([]projectChange, 0, len(changes))
	for _, change := range changes {
		if err := replaceFile(change.path, change.updated); err != nil {
			for index := len(updated) - 1; index >= 0; index-- {
				_ = replaceFile(updated[index].path, updated[index].original)
			}
			return fmt.Errorf("更新项目文件: %w", err)
		}
		updated = append(updated, change)
	}
	return nil
}
