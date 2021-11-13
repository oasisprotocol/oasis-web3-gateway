package model

import (
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

// InitModel initializes db models.
func InitModel(db *pg.DB) error {
	models := []interface{}{
		new(Block),
		new(BlockRef),
		new(TransactionRef),
		new(Transaction),
		new(IndexedRoundWithTip),
		new(Log),
	}

	for _, m := range models {
		if err := db.Model(m).CreateTable(&orm.CreateTableOptions{IfNotExists: true}); err != nil {
			return err
		}
	}

	return nil
}
