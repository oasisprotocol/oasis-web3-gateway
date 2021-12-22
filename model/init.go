package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"

	"github.com/oasisprotocol/oasis-evm-web3-gateway/model/migrations"
)

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

// Migrate migrates the DB to latest version.
func Migrate(ctx context.Context, db *bun.DB) error {
	logger := logging.GetLogger("migration")

	// Initialize the migrator.
	migrator := migrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		logger.Error("failed to init migrations", "err", err)
		return err
	}

	// Run migrations.
	_, err := migrator.Migrate(ctx)
	if err != nil {
		logger.Error("failed to migrate db", "err", err)
		return err
	}

	status, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return err
	}
	logger.Info("migration done", "applied", status.Applied(), "last_group", status.LastGroupID(), "status", status.String())

	return nil
}
