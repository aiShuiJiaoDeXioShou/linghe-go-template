package devtool

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"go-template/internal/config"
)

var gitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type releaseOptions struct {
	environment string
	sha         string
	goarch      string
	output      string
	dryRun      bool
}

type releaseManifest struct {
	Name        string `json:"name"`
	Environment string `json:"environment"`
	Commit      string `json:"commit"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
}

func (c command) packageRelease(ctx context.Context, args []string) error {
	options, err := parseReleaseOptions(args)
	if err != nil {
		return err
	}
	if options.sha == "" {
		options.sha, err = currentGitSHA(ctx, c.root)
		if err != nil {
			return err
		}
	}
	if !gitSHAPattern.MatchString(options.sha) {
		return fmt.Errorf("发布版本必须是完整的小写 Git SHA")
	}

	configPath := filepath.Join(c.root, "configs", "config."+options.environment+".yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if cfg.App.Env != options.environment {
		return fmt.Errorf("%s 的 app.env 必须是 %s", relative(c.root, configPath), options.environment)
	}
	if _, err := validateMigrations(filepath.Join(c.root, "migrations")); err != nil {
		return err
	}
	composePath := filepath.Join(c.root, "deploy", "docker-compose."+options.environment+".yaml")
	if options.output == "" {
		options.output = filepath.Join("dist", fmt.Sprintf("%s-%s-%s.tar.gz", cfg.App.Name, options.environment, options.sha))
	}
	outputPath := options.output
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(c.root, outputPath)
	}
	if filepath.Ext(outputPath) != ".gz" || !strings.HasSuffix(outputPath, ".tar.gz") {
		return fmt.Errorf("发布包输出路径必须以 .tar.gz 结尾")
	}
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("发布包已存在: %s", outputPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查发布包路径: %w", err)
	}

	if options.dryRun {
		_, _ = fmt.Fprintf(c.stdout, "release env=%s sha=%s goarch=%s output=%s\n", options.environment, options.sha, options.goarch, outputPath)
		_, _ = fmt.Fprintf(c.stdout, "include server Dockerfile docker-compose.yml release.json configs/%s migrations/*.sql\n", filepath.Base(configPath))
		return nil
	}
	if err := c.runProcess(ctx, nil, "docker", "compose", "-f", composePath, "config", "--quiet"); err != nil {
		return err
	}

	stagingRoot, err := os.MkdirTemp("", cfg.App.Name+"-release-*")
	if err != nil {
		return fmt.Errorf("创建发布临时目录: %w", err)
	}
	defer os.RemoveAll(stagingRoot)
	if err := stageRelease(c.root, stagingRoot, configPath, composePath, releaseManifest{
		Name:        cfg.App.Name,
		Environment: options.environment,
		Commit:      options.sha,
		GOOS:        "linux",
		GOARCH:      options.goarch,
	}); err != nil {
		return err
	}
	serverPath := filepath.Join(stagingRoot, "server")
	buildEnvironment := []string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + options.goarch}
	if err := c.runProcess(ctx, buildEnvironment, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", serverPath, "."); err != nil {
		return err
	}
	if err := os.Chmod(serverPath, 0o755); err != nil {
		return fmt.Errorf("设置服务文件权限: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("创建发布包目录: %w", err)
	}
	if err := archiveRelease(stagingRoot, outputPath); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.stdout, "archive=%s\n", outputPath)
	return nil
}

func parseReleaseOptions(args []string) (releaseOptions, error) {
	options := releaseOptions{goarch: "amd64"}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--env", "--sha", "--goarch", "--output":
			index++
			if index >= len(args) {
				return releaseOptions{}, fmt.Errorf("%s 缺少值", argument)
			}
			value := strings.TrimSpace(args[index])
			switch argument {
			case "--env":
				options.environment = value
			case "--sha":
				options.sha = value
			case "--goarch":
				options.goarch = value
			case "--output":
				options.output = value
			}
		case "--dry-run":
			options.dryRun = true
		default:
			return releaseOptions{}, fmt.Errorf("未知参数 %q", argument)
		}
	}
	if options.environment != "stg" && options.environment != "production" {
		return releaseOptions{}, fmt.Errorf("--env 只允许 stg 或 production")
	}
	if options.goarch != "amd64" && options.goarch != "arm64" {
		return releaseOptions{}, fmt.Errorf("--goarch 只允许 amd64 或 arm64 当前运行架构为 %s", runtime.GOARCH)
	}
	return options, nil
}

func currentGitSHA(ctx context.Context, root string) (string, error) {
	process := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	process.Dir = root
	output, err := process.Output()
	if err != nil {
		return "", fmt.Errorf("读取 Git 版本: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func stageRelease(root string, stagingRoot string, configPath string, composePath string, manifest releaseManifest) error {
	if err := os.MkdirAll(filepath.Join(stagingRoot, "configs"), 0o755); err != nil {
		return fmt.Errorf("创建发布配置目录: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(stagingRoot, "migrations"), 0o755); err != nil {
		return fmt.Errorf("创建发布迁移目录: %w", err)
	}
	files := [][2]string{
		{filepath.Join(root, "deploy", "Dockerfile"), filepath.Join(stagingRoot, "Dockerfile")},
		{composePath, filepath.Join(stagingRoot, "docker-compose.yml")},
		{configPath, filepath.Join(stagingRoot, "configs", filepath.Base(configPath))},
	}
	for _, file := range files {
		if err := copyFile(file[0], file[1]); err != nil {
			return err
		}
	}
	migrations, err := filepath.Glob(filepath.Join(root, "migrations", "*.sql"))
	if err != nil {
		return fmt.Errorf("扫描迁移文件: %w", err)
	}
	sort.Strings(migrations)
	for _, source := range migrations {
		if err := copyFile(source, filepath.Join(stagingRoot, "migrations", filepath.Base(source))); err != nil {
			return err
		}
	}
	manifestContent, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("生成发布清单: %w", err)
	}
	manifestContent = append(manifestContent, '\n')
	if err := os.WriteFile(filepath.Join(stagingRoot, "release.json"), manifestContent, 0o644); err != nil {
		return fmt.Errorf("写入发布清单: %w", err)
	}
	return nil
}

func copyFile(source string, target string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("读取 %s: %w", source, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("读取 %s 元数据: %w", source, err)
	}
	if err := os.WriteFile(target, content, info.Mode().Perm()); err != nil {
		return fmt.Errorf("写入 %s: %w", target, err)
	}
	return nil
}

func archiveRelease(stagingRoot string, outputPath string) (err error) {
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("创建发布包: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("关闭发布包: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()

	gzipWriter := gzip.NewWriter(file)
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	tarWriter := tar.NewWriter(gzipWriter)
	paths := make([]string, 0)
	if walkErr := filepath.WalkDir(stagingRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != stagingRoot {
			paths = append(paths, path)
		}
		return nil
	}); walkErr != nil {
		return fmt.Errorf("扫描发布内容: %w", walkErr)
	}
	sort.Strings(paths)
	for _, path := range paths {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return fmt.Errorf("读取发布文件: %w", statErr)
		}
		header, headerErr := tar.FileInfoHeader(info, "")
		if headerErr != nil {
			return fmt.Errorf("生成归档头: %w", headerErr)
		}
		relativePath, relativeErr := filepath.Rel(stagingRoot, path)
		if relativeErr != nil {
			return fmt.Errorf("解析发布路径: %w", relativeErr)
		}
		header.Name = filepath.ToSlash(relativePath)
		if info.IsDir() {
			header.Name += "/"
		}
		header.ModTime = time.Unix(0, 0)
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""
		if writeErr := tarWriter.WriteHeader(header); writeErr != nil {
			return fmt.Errorf("写入归档头: %w", writeErr)
		}
		if info.IsDir() {
			continue
		}
		source, openErr := os.Open(path)
		if openErr != nil {
			return fmt.Errorf("打开发布文件: %w", openErr)
		}
		_, copyErr := io.Copy(tarWriter, source)
		closeErr := source.Close()
		if copyErr != nil {
			return fmt.Errorf("写入发布文件: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("关闭发布文件: %w", closeErr)
		}
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("完成 tar 归档: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("完成 gzip 压缩: %w", err)
	}
	return nil
}
