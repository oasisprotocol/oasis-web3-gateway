package migrations

import (
	"embed"

	"github.com/uptrace/bun/migrate"
)

// Migrations are all migrations.
var Migrations *migrate.Migrations

//go:embed *.sql
var migrations embed.FS

func init() {
	Migrations = migrate.NewMigrations()
	for _, m := range []migrate.Migration{
		// Initial.
		{
			Name: "20211213143752",
			Up:   migrate.NewSQLMigrationFunc(migrations, "20211213143752_initial.up.sql"),
		},
	} {
		Migrations.Add(m)
	}
}
