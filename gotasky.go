// Command gotasky combines template snippets into a Taskfile.yml for go-task.
package main

import (
	"os"

	"github.com/joho/godotenv"

	"github.com/sig9org/gotasky/internal/cli"
)

func main() {
	_ = godotenv.Load()
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
