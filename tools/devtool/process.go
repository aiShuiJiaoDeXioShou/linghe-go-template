package devtool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (c command) runProcess(ctx context.Context, environment []string, arguments ...string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("缺少待执行命令")
	}
	_, _ = fmt.Fprintf(c.stdout, "> %s\n", strings.Join(arguments, " "))
	process := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	process.Dir = c.root
	process.Stdout = c.stdout
	process.Stderr = c.stderr
	process.Env = mergedEnvironment(environment)
	if err := process.Run(); err != nil {
		return fmt.Errorf("执行 %s: %w", strings.Join(arguments, " "), err)
	}
	return nil
}

func mergedEnvironment(overrides []string) []string {
	keys := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		if separator := strings.IndexByte(value, '='); separator > 0 {
			keys[value[:separator]] = true
		}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		separator := strings.IndexByte(value, '=')
		if separator > 0 && keys[value[:separator]] {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, overrides...)
}
