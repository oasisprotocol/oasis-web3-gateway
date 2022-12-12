package migrations

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/uptrace/bun"

	"github.com/oasisprotocol/oasis-web3-gateway/db/migrator"
)

// Migrations are all migrations.
var Migrations *migrator.Migrations

//go:embed *.sql
var migrations embed.FS

// DropTables deletes all database tables in the `public` schema of the configured database.
//
// Note: this method assumes that PostgresSQL is used as the underlying db.
func DropTables(ctx context.Context, db *bun.DB) error {
	logger := logging.GetLogger("migration")

	rows, err := db.QueryContext(ctx, "SELECT * FROM pg_tables")
	if err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}

	var results []map[string]interface{}
	if err = db.ScanRows(ctx, rows, &results); err != nil {
		return err
	}

	for _, result := range results {
		tableName := string(result["tablename"].([]byte))
		schemaName := string(result["schemaname"].([]byte))

		// Skip internal pg tables.
		if strings.Compare(schemaName, "public") != 0 {
			continue
		}

		logger.Info("dropping table", "table_name", tableName)

		_, err := db.ExecContext(ctx, fmt.Sprintf("DROP TABLE \"%s\" CASCADE", tableName))
		if err != nil {
			logger.Error("failed dropping table", "table_name", tableName, "err", err)
			continue
		}
	}

	return nil
}

// Init initializes the migrator tables.
func Init(ctx context.Context, db *bun.DB) error {
	// Initialize the migrator.
	migrator := migrator.NewMigrator(db, Migrations)
	return migrator.Init(ctx)
}

// Migrate migrates the DB to latest version.
func Migrate(ctx context.Context, db *bun.DB) error {
	logger := logging.GetLogger("migration")

	// Initialize the migrator.
	migrator := migrator.NewMigrator(db, Migrations)

	// Run migrations.
	if err := migrator.Migrate(ctx); err != nil {
		logger.Error("failed to migrate db", "err", err)
		return err
	}

	status, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return err
	}
	logger.Info("migration done", "applied", status.Applied(), "status", status.String(), "unaplied", status.Unapplied())

	return nil
}

func init() {
	Migrations = migrator.NewMigrations()
	for _, m := range []migrator.Migration{
		// Initial.
		{
			Name: "20211213143752",
			Up:   migrator.NewSQLMigrationFunc(migrations, "20211213143752_initial.up.sql"),
		},
		// Logs index fix migration.
		// https://github.com/oasisprotocol/oasis-web3-gateway/pull/174
		{
			Name: "20220109122505",
			Up:   LogsUp,
		},
		// Add missing receipt on round index, used in pruning.
		// https://github.com/oasisprotocol/oasis-web3-gateway/issues/227
		{
			Name: "20220324091030",
			Up:   migrator.NewSQLMigrationFunc(migrations, "20220324091030_receipt_round_index.up.sql"),
		},
	} {
		Migrations.Add(m)
	}
}
