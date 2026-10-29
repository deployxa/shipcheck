// Command shipcheck runs production-readiness checks over an application
// directory. Static analysis mode never executes project code, never needs
// Docker or network access, and works fully offline.
package main

import (
	"os"

	"github.com/deployxa/shipcheck/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
