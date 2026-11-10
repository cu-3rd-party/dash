package main

import (
	"context"
	"fmt"
	"os"

	"git.cu3rd.ru/cu3rd/dash/cmd"
)

func main() {
	if err := cmd.NewMainApp().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
