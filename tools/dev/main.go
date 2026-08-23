package main

import (
	"context"
	"fmt"
	"os"

	"go-template/tools/devtool"
)

func main() {
	if err := devtool.Run(context.Background(), "", os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
