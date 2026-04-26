package main

import (
	"fmt"
	"os"

	"github.com/penwyp/typelens/internal/cli"
)

func main() {
	root, err := cli.NewRootCommand()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
