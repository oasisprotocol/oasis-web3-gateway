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
func CreateTables(db *bun.DB) error {
	for _, tb := range tables {
		_, err := db.NewCreateTable().Model(tb).IfNotExists().Exec(context.Background())
		if err != nil {
			return err
		}
	}

	return nil
}

// TruncateModel clears any DB records.
func TruncateModel(db *bun.DB) error {
	for _, tb := range tables {
		if _, err := db.NewDropTable().Model(tb).IfExists().Exec(context.Background()); err != nil {
			return err
		}
	}

	return nil
}
