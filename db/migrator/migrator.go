// Package migrator implements a custom migrator inspired by the upstram [migrator](https://github.com/uptrace/bun/blob/master/migrate/migrator.go).
//
// The upstream migrator first marks the migration as applied and then only after runs it.
// This migrator runs both the migration and the "successful mark" in a transaction. Therefore if
// the migration is aborted (or fails) the transaction is not applied and no manual rollback is needed.
package migrator

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

const (
	table = "bun_migrations"
)

// Migrator is database migrator.
type Migrator struct {
	db         *bun.DB
	migrations *Migrations
}

// NewMigrator initializes the migrator.
func NewMigrator(db *bun.DB, migrations *Migrations) *Migrator {
	m := &Migrator{
		db:         db,
		migrations: migrations,
	}
	return m
}

// Init initializes the migration tables.
func (m *Migrator) Init(ctx context.Context) error {
	if _, err := m.db.NewCreateTable().
		Model((*Migration)(nil)).
		ModelTableExpr(table).
		IfNotExists().
		Exec(ctx); err != nil {
		return err
	}
	return nil
}

// Delete deletes the migration tables.
func (m *Migrator) Delete(ctx context.Context) error {
	if _, err := m.db.NewDropTable().
		Model((*Migration)(nil)).
		ModelTableExpr(table).
		IfExists().
		Exec(ctx); err != nil {
		return err
	}
	return nil
}

// Migrate runs unapplied migrations.
func (m *Migrator) Migrate(ctx context.Context) error {
	if len(m.migrations.Sorted()) == 0 {
		return fmt.Errorf("no migrations")
	}
	var stop bool
	for {
		if stop {
			break
		}
		// Apply next migration in a transaction.
		if err := m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// Lock migrations table for the transaction duration.
			if err := lockMigrations(ctx, &tx); err != nil {
				return err
			}

			// Fetch remaining migrations.
			migrations, err := migrationsWithStatus(ctx, &tx, m.migrations)
			if err != nil {
				return err
			}
			migrations = migrations.Unapplied()

			if len(migrations) == 0 {
				// No more migrations.
				stop = true
				return nil
			}

			// Apply migration.
			migration := &migrations[0]
			if migration.Up != nil {
				if err = migration.Up(ctx, &tx); err != nil {
					return err
				}
			}
			if err = markApplied(ctx, &tx, migration); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return fmt.Errorf("applying migration: %w", err)
		}
	}

	return nil
}

// Rollback rollbacks last applied migration.
func (m *Migrator) Rollback(ctx context.Context) error {
	if len(m.migrations.Sorted()) == 0 {
		return fmt.Errorf("no migrations")
	}

	// Rollback latest migration in a transaction.
	return m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Lock migrations table for the transaction duration.
		if err := lockMigrations(ctx, &tx); err != nil {
			return err
		}

		// Fetch migrations.
		migrations, err := migrationsWithStatus(ctx, &tx, m.migrations)
		if err != nil {
			return err
		}
		applied := migrations.Applied()

		if len(applied) == 0 {
			return fmt.Errorf("no applied migrations")
		}

		// Apply migration.
		migration := &applied[0]
		if migration.Down != nil {
			if err = migration.Down(ctx, &tx); err != nil {
				return err
			}
		}
		if err = markUnapplied(ctx, &tx, migration); err != nil {
			return err
		}

		return nil
	})
}

// MigrationsWithStatus returns migrations with status in ascending order.
func (m *Migrator) MigrationsWithStatus(ctx context.Context) (MigrationSlice, error) {
	return migrationsWithStatus(ctx, m.db, m.migrations)
}

// migrationsWithStatus returns migrations with status in ascending order.
func migrationsWithStatus(ctx context.Context, db bun.IDB, migrations *Migrations) (MigrationSlice, error) {
	// Fetch applied transactions.
	var applied MigrationSlice
	if err := db.NewSelect().
		ColumnExpr("*").
		Model(&applied).
		ModelTableExpr(table).
		Scan(ctx); err != nil {
		return nil, err
	}

	// Create a map of applied migrations.
	appliedMap := make(map[string]*Migration, len(applied))
	for i := range applied {
		m := &applied[i]
		appliedMap[m.Name] = m
	}

	// Fetch migrations statuses.
	sorted := migrations.Sorted()
	for i := range sorted {
		m1 := &sorted[i]
		if m2, ok := appliedMap[m1.Name]; ok {
			m1.ID = m2.ID
			m1.MigratedAt = m2.MigratedAt
		}
	}

	return sorted, nil
}

// lockMigrations obtains a lock of the migrations table.
func lockMigrations(ctx context.Context, db *bun.Tx) error {
	if _, err := db.ExecContext(ctx, fmt.Sprintf("LOCK TABLE %s", table)); err != nil {
		return fmt.Errorf("locking migrations: %w", err)
	}
	return nil
}

// markApplied marks the migration as applied (completed).
func markApplied(ctx context.Context, db bun.IDB, migration *Migration) error {
	_, err := db.NewInsert().Model(migration).
		ModelTableExpr(table).
		Exec(ctx)
	return err
}

// markUnapplied marks the migration as unapplied (new).
func markUnapplied(ctx context.Context, db bun.IDB, migration *Migration) error {
	_, err := db.NewDelete().
		Model(migration).
		ModelTableExpr(table).
		Where("id = ?", migration.ID).
		Exec(ctx)
	return err
}
