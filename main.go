package main

import (
	"github.com/allintech/github-sentry/cmd"
)

func main() {
	// Use cobra for all command handling
	// Default behavior (no args) runs the server
	cmd.Execute()
}
