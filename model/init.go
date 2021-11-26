package model

import (
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

var models = []interface{}{
	new(AccessTuple),
	new(Block),
	new(BlockRef),
	new(Header),
	new(TransactionRef),
	new(Transaction),
	new(IndexedRoundWithTip),
	new(Log),
	new(Receipt),
}

// InitModel initializes db models.
func InitModel(db *pg.DB) error {
	for _, m := range models {
		if err := db.Model(m).CreateTable(&orm.CreateTableOptions{IfNotExists: true}); err != nil {
			return err
		}
	}

	return nil
}

// TruncateModel clears any DB records.
func TruncateModel(db *pg.DB) error {
	for _, m := range models {
		if err := db.Model(m).DropTable(&orm.DropTableOptions{
			IfExists: true,
			Cascade:  true,
		}); err != nil {
			return err
		}
	}

	return nil
}
