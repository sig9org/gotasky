// Command gotasky combines template snippets into a Taskfile.yml for go-task.
package main

import (
	"os"

	"github.com/sig9org/gotasky/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
