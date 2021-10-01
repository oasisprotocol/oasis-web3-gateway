package model

import (
	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

var Models []interface{}

// Add model to Models
func addModel() {
	Models = append(Models, new(BlockRef))
	Models = append(Models, new(TransactionRef))
}

// InitModel initializes models
func InitModel(db *pg.DB) error {
	addModel()
	for _, m := range Models {
		if err := db.Model(m).CreateTable(&orm.CreateTableOptions{}); err != nil {
			return err
		}
	}
	return nil
}
