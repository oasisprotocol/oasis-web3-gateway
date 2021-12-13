package model

import (
	"context"

	"github.com/uptrace/bun"
)

var tables = []interface{}{
	new(AccessTuple),
	new(Block),
	new(BlockRef),
	new(Header),
	new(Transaction),
	new(TransactionRef),
	new(IndexedRoundWithTip),
	new(Receipt),
	new(Log),
}

// CreateTables creates tables.
func CreateTables(ctx context.Context, db *bun.DB) error {
	for _, tb := range tables {
		_, err := db.NewCreateTable().Model(tb).IfNotExists().Exec(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// TruncateModel clears any DB records.
func TruncateModel(ctx context.Context, db *bun.DB) error {
	for _, tb := range tables {
		if _, err := db.NewDropTable().Model(tb).IfExists().Exec(ctx); err != nil {
			return err
		}
	}

	return nil
}
