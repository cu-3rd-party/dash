package cmd

import (
	"context"
	"io"
	"os"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	backup_service "git.cu3rd.ru/cu3rd/dash/services/backup"

	"github.com/urfave/cli/v3"
)

// cmdExport dumps the database configured by DASH_DB_* as JSON, for the import
// form on the settings page of another instance:
//
//	DASH_DB_DSN=data/finance.db dash export -o dash.json
func cmdExport() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Dump categories, expenses, payments, donations and settings as JSON",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "file to write (default: stdout)"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			setting.Load()
			if err := db.InitEngine(ctx); err != nil {
				return err
			}
			d, err := backup_service.Export(ctx)
			if err != nil {
				return err
			}
			var w io.Writer = os.Stdout
			if path := c.String("output"); path != "" {
				f, err := os.Create(path)
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				w = f
			}
			return backup_service.Write(w, d)
		},
	}
}
