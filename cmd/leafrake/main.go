// Command leafrake classifies local git branches — merged, squash-merged,
// stale, gone — and deletes the dead ones with per-branch evidence.
package main

import (
	"os"

	"github.com/JaydenCJ/leafrake/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
