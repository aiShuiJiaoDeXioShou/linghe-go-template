package devtool

import (
	"context"
)

func (c command) check(ctx context.Context) error {
	commands := [][]string{
		{"go", "test", "./..."},
		{"go", "vet", "./..."},
		{"go", "build", "./..."},
	}
	for _, arguments := range commands {
		if err := c.runProcess(ctx, nil, arguments...); err != nil {
			return err
		}
	}
	return nil
}
