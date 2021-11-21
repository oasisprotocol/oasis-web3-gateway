package model

import (
	"context"
	"github.com/uptrace/bun"
)

// CreateTables creates tables.
func CreateTables(db *bun.DB) {
	// create tables
	create := db.NewCreateTable()
	create.Model(new(Block)).Exec(context.Background())
	create.Model(new(BlockRef)).Exec(context.Background())
	create.Model(new(TransactionRef)).Exec(context.Background())
	create.Model(new(Transaction)).Exec(context.Background())
	create.Model(new(ContinuesIndexedRound)).Exec(context.Background())
	create.Model(new(IndexedRoundWithTip)).Exec(context.Background())
	create.Model(new(Receipt)).Exec(context.Background())
	create.Model(new(Log)).Exec(context.Background())
}
