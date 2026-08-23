package devtool

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	migrationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	migrationFilePattern = regexp.MustCompile(`^(\d{6})_([a-z][a-z0-9_]*)\.(up|down)\.sql$`)
)

type migrationOptions struct {
	name   string
	dryRun bool
}

type migrationSummary struct {
	latest uint
	count  int
}

func (c command) newMigration(args []string) error {
	options, err := parseMigrationOptions(args)
	if err != nil {
		return err
	}
	migrationsDirectory := filepath.Join(c.root, "migrations")
	summary, err := validateMigrations(migrationsDirectory)
	if err != nil {
		return err
	}
	version := int(summary.latest) + 1
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

func (c command) checkMigrations() error {
	summary, err := validateMigrations(filepath.Join(c.root, "migrations"))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.stdout, "迁移检查通过 versions=%d latest=%06d\n", summary.count, summary.latest)
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

func validateMigrations(directory string) (migrationSummary, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return migrationSummary{}, nil
		}
		return migrationSummary{}, fmt.Errorf("读取 migrations 目录: %w", err)
	}
	type migrationPair struct {
		name       string
		directions map[string]bool
	}
	pairs := make(map[uint]*migrationPair)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return migrationSummary{}, fmt.Errorf("迁移文件名不符合约定: %s", entry.Name())
		}
		versionNumber, conversionErr := strconv.ParseUint(matches[1], 10, 32)
		if conversionErr != nil {
			return migrationSummary{}, fmt.Errorf("解析迁移版本 %q: %w", entry.Name(), conversionErr)
		}
		version := uint(versionNumber)
		if version == 0 {
			return migrationSummary{}, fmt.Errorf("迁移版本必须从 000001 开始: %s", entry.Name())
		}
		pair := pairs[version]
		if pair == nil {
			pair = &migrationPair{name: matches[2], directions: make(map[string]bool)}
			pairs[version] = pair
		}
		if pair.name != matches[2] {
			return migrationSummary{}, fmt.Errorf("迁移版本 %06d 使用了不同名称 %q 和 %q", version, pair.name, matches[2])
		}
		direction := matches[3]
		if pair.directions[direction] {
			return migrationSummary{}, fmt.Errorf("迁移版本 %06d 存在重复的 %s 文件", version, direction)
		}
		pair.directions[direction] = true
	}

	versions := make([]int, 0, len(pairs))
	for version := range pairs {
		versions = append(versions, int(version))
	}
	sort.Ints(versions)
	for _, version := range versions {
		pair := pairs[uint(version)]
		if !pair.directions["up"] || !pair.directions["down"] {
			return migrationSummary{}, fmt.Errorf("迁移版本 %06d 必须同时包含 up 和 down 文件", version)
		}
	}
	if len(versions) == 0 {
		return migrationSummary{}, nil
	}
	return migrationSummary{latest: uint(versions[len(versions)-1]), count: len(versions)}, nil
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
