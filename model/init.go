package model

import (
	"github.com/uptrace/bun"
)

// RegisterModel initializes db models.
func RegisterModel(db *bun.DB) {
	models := []interface{}{
		new(BlockRef),
		new(TransactionRef),
		new(Transaction),
		new(ContinuesIndexedRound)}

	// register model
	db.RegisterModel(models)
}
