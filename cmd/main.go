// Package cmd holds the CLI entrypoints.
package cmd

import "github.com/urfave/cli/v3"

// NewMainApp builds the root CLI command.
func NewMainApp() *cli.Command {
	return &cli.Command{
		Name:  "dash",
		Usage: "Расходы и донаты cu3rd",
		Commands: []*cli.Command{
			cmdWeb(),
			cmdMigrate(),
			cmdExport(),
		},
		DefaultCommand: "web",
	}
}
