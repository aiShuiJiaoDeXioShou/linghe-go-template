package devtool

import (
	"context"
	"os"
	"path/filepath"
)

const apiDocsDirectory = "docs/swagger"

func (c command) generateAPIDocs(ctx context.Context) error {
	return c.runAPIDocsGenerator(ctx, filepath.Join(c.root, apiDocsDirectory))
}

func apiDocsConfigured(root string) bool {
	info, err := os.Stat(filepath.Join(root, apiDocsDirectory))
	return err == nil && info.IsDir()
}

func (c command) runAPIDocsGenerator(ctx context.Context, output string) error {
	return c.runProcess(ctx, nil,
		"go", "tool", "swag", "init",
		"-g", "main.go",
		"-d", ".",
		"--parseInternal",
		"--exclude", apiDocsDirectory,
		"-o", output,
		"--packageName", "swagger",
		"--quiet",
	)
}
