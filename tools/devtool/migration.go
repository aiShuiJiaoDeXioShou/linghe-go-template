package devtool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	migrationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	migrationFilePattern = regexp.MustCompile(`^(\d{6})_[a-z][a-z0-9_]*\.(up|down)\.sql$`)
)

type migrationOptions struct {
	name   string
	dryRun bool
}

func (c command) newMigration(args []string) error {
	options, err := parseMigrationOptions(args)
	if err != nil {
		return err
	}
	migrationsDirectory := filepath.Join(c.root, "migrations")
	version, err := nextMigrationVersion(migrationsDirectory)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("%06d_%s", version, options.name)
	upPath := filepath.Join(migrationsDirectory, prefix+".up.sql")
	downPath := filepath.Join(migrationsDirectory, prefix+".down.sql")
	upContent := []byte(fmt.Sprintf("BEGIN;\n\n-- 编写 %s 的升级 SQL\n\nCOMMIT;\n", options.name))
	downContent := []byte(fmt.Sprintf("BEGIN;\n\n-- 编写 %s 的回滚 SQL\n\nCOMMIT;\n", options.name))

	if options.dryRun {
		previewFile(c.stdout, c.root, upPath, upContent)
		previewFile(c.stdout, c.root, downPath, downContent)
		return nil
	}
	if err := os.MkdirAll(migrationsDirectory, 0o755); err != nil {
		return fmt.Errorf("创建 migrations 目录: %w", err)
	}
	if err := writeExclusive(upPath, upContent); err != nil {
		return err
	}
	if err := writeExclusive(downPath, downContent); err != nil {
		_ = os.Remove(upPath)
		return err
	}
	_, _ = fmt.Fprintf(c.stdout, "created %s\ncreated %s\n", relative(c.root, upPath), relative(c.root, downPath))
	return nil
}

func parseMigrationOptions(args []string) (migrationOptions, error) {
	options := migrationOptions{}
	for _, argument := range args {
		switch argument {
		case "--dry-run":
			options.dryRun = true
		default:
			if len(argument) > 0 && argument[0] == '-' {
				return migrationOptions{}, fmt.Errorf("未知参数 %q", argument)
			}
			if options.name != "" {
				return migrationOptions{}, fmt.Errorf("migration new 只接受一个名称")
			}
			options.name = argument
		}
	}
	if !migrationNamePattern.MatchString(options.name) {
		return migrationOptions{}, fmt.Errorf("迁移名称必须匹配 %s", migrationNamePattern.String())
	}
	return options, nil
}

func nextMigrationVersion(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("读取 migrations 目录: %w", err)
	}
	maximum := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, conversionErr := strconv.Atoi(matches[1])
		if conversionErr != nil {
			return 0, fmt.Errorf("解析迁移版本 %q: %w", entry.Name(), conversionErr)
		}
		if version > maximum {
			maximum = version
		}
	}
	return maximum + 1, nil
}

func writeExclusive(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("创建 %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("写入 %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("关闭 %s: %w", path, err)
	}
	return nil
}

func relative(root string, path string) string {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relativePath)
}
