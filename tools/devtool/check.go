package devtool

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func (c command) check(ctx context.Context) error {
	commands := [][]string{
		{"go", "fmt", "./..."},
		{"go", "test", "./..."},
		{"go", "vet", "./..."},
		{"go", "build", "./..."},
	}
	for _, arguments := range commands {
		_, _ = fmt.Fprintf(c.stdout, "> %s\n", strings.Join(arguments, " "))
		process := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
		process.Dir = c.root
		process.Stdout = c.stdout
		process.Stderr = c.stderr
		if err := process.Run(); err != nil {
			return fmt.Errorf("执行 %s: %w", strings.Join(arguments, " "), err)
		}
	}
	return nil
}
