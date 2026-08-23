// devtool 包提供项目开发流程使用的确定性命令
package devtool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const usage = `用法:
  go run ./tools/dev check
  go run ./tools/dev docs generate
  go run ./tools/dev project init --module <module-path> --name <project-name> [--dry-run]
  go run ./tools/dev migration new <name> [--dry-run]
  go run ./tools/dev migration check
  go run ./tools/dev module new <domain> [--realm app|admin|none] [--dry-run]
  go run ./tools/dev release package --env stg|production [--sha <git-sha>] [--goarch amd64|arm64] [--output <archive>] [--dry-run]
  go run ./tools/dev test integration
`

type command struct {
	root   string
	stdout io.Writer
	stderr io.Writer
}

// Run 执行项目开发命令 root 为空时自动向上查找 go.mod
func Run(
	ctx context.Context,
	root string,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = io.WriteString(stdout, usage)
		return nil
	}

	projectRoot, err := resolveProjectRoot(root)
	if err != nil {
		return err
	}
	runner := command{root: projectRoot, stdout: stdout, stderr: stderr}

	switch args[0] {
	case "check":
		if len(args) != 1 {
			return fmt.Errorf("check 不接受额外参数\n%s", usage)
		}
		return runner.check(ctx)
	case "docs":
		if len(args) != 2 || args[1] != "generate" {
			return fmt.Errorf("docs 仅支持 generate 子命令\n%s", usage)
		}
		return runner.generateAPIDocs(ctx)
	case "project":
		if len(args) < 2 || args[1] != "init" {
			return fmt.Errorf("project 仅支持 init 子命令\n%s", usage)
		}
		return runner.initProject(ctx, args[2:])
	case "migration":
		if len(args) < 2 {
			return fmt.Errorf("migration 必须指定 new 或 check 子命令\n%s", usage)
		}
		switch args[1] {
		case "new":
			return runner.newMigration(args[2:])
		case "check":
			if len(args) != 2 {
				return fmt.Errorf("migration check 不接受额外参数")
			}
			return runner.checkMigrations()
		default:
			return fmt.Errorf("migration 仅支持 new 或 check 子命令\n%s", usage)
		}
	case "module":
		if len(args) < 2 || args[1] != "new" {
			return fmt.Errorf("module 仅支持 new 子命令\n%s", usage)
		}
		return runner.newModule(args[2:])
	case "release":
		if len(args) < 2 || args[1] != "package" {
			return fmt.Errorf("release 仅支持 package 子命令\n%s", usage)
		}
		return runner.packageRelease(ctx, args[2:])
	case "test":
		if len(args) != 2 || args[1] != "integration" {
			return fmt.Errorf("test 仅支持 integration 子命令\n%s", usage)
		}
		return runner.testIntegration(ctx)
	default:
		return fmt.Errorf("未知命令 %q\n%s", args[0], usage)
	}
}

func resolveProjectRoot(root string) (string, error) {
	if root == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("读取当前目录: %w", err)
		}
		root = workingDirectory
	}

	current, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("解析项目目录: %w", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("未找到 go.mod")
		}
		current = parent
	}
}

func modulePath(root string) (string, error) {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("打开 go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if path != "" {
				return path, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取 go.mod: %w", err)
	}
	return "", fmt.Errorf("go.mod 缺少 module 声明")
}

func previewFile(writer io.Writer, root string, path string, content []byte) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		relativePath = path
	}
	_, _ = fmt.Fprintf(writer, "--- %s\n%s", filepath.ToSlash(relativePath), content)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		_, _ = io.WriteString(writer, "\n")
	}
}
