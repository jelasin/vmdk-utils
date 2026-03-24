package main

import (
	"fmt"
	"os"

	"github.com/jelasin/vmdk-utils/internal/cli"
)

func main() {
	app := cli.NewApp(os.Stdout, os.Stderr)
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
