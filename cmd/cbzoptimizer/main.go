package main

import (
	"github.com/belphemur/CBZOptimizer/v2/cmd/cbzoptimizer/commands"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	commands.SetVersionInfo(version, commit, date)

	// Configure logging before executing commands
	commands.ConfigureLogging()

	commands.Execute()
}
