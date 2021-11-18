package model

import (
	"github.com/uptrace/bun"
)

// RegisterModel initializes db models.
func RegisterModel(db *bun.DB) {
	// register model
	db.RegisterModel(new(BlockRef))
	db.RegisterModel(new(TransactionRef))
	db.RegisterModel(new(Transaction))
	db.RegisterModel(new(ContinuesIndexedRound))
}
