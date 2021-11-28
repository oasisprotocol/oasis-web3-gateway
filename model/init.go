package model

import (
	"context"
	"github.com/uptrace/bun"
)


// CreateTables creates tables.
func CreateTables(db *bun.DB) error {
	var err error
	// tables
	tables := []interface{}{
		new(Block),
		new(BlockRef),
		new(Transaction),
		new(TransactionRef),
		new(IndexedRoundWithTip),
		new(Receipt),
		new(Log),
	}

	// create tables
	for _, tb := range tables {
		_, err = db.NewCreateTable().Model(tb).IfNotExists().Exec(context.Background())
		if err != nil {
			return err
		}
	}

	return nil
}
