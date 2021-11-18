package psql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type PostDb struct {
	Db *bun.DB
}

// InitDb creates postgresql db instance
func InitDb(cfg *conf.DatabaseConfig) (*PostDb, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}

	// dsn
	dsn := fmt.Sprintf("postgres://%v:%v@%v:%v/%v?sslmode=disable&dial_timeout=%v&read_timeout=%v&write_timeout=%v",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Db, cfg.DialTimeout, cfg.ReadTimeout, cfg.WriteTimeout)

	// open
	sqlDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	// create db
	db := bun.NewDB(sqlDB, pgdialect.New())

	// register model
	model.RegisterModel(db)

	return &PostDb{Db: db}, nil
}

// GetTransactionRef returns block hash, round and index of the transaction.
func (db *PostDb) GetTransactionRef(txHash string) (*model.TransactionRef, error) {
	tx := new(model.TransactionRef)
	err := db.Db.NewSelect().Model(tx).Where("eth_tx_hash =? ", txHash).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// GetTransaction queries ethereum transaction by hash.
func (db *PostDb) GetTransaction(hash string) (*model.Transaction, error) {
	tx := new(model.Transaction)
	err := db.Db.NewSelect().Model(tx).Where("hash = ?", hash).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// Store stores data.
func (db *PostDb) Store(value interface{}) error {
	_, err := db.Db.NewInsert().Model(value).Exec(context.Background())
	return err
}

// Update updates record.
func (db *PostDb) Update(value interface{}) error {
	l, err := db.Db.NewSelect().Model(value).Count(context.Background())
	if err != nil {
		return err
	}
	if l == 0 {
		_, err = db.Db.NewInsert().Model(value).Exec(context.Background())
	} else {
		_, err = db.Db.NewUpdate().Model(value).Exec(context.Background())
	}

	return err
}

// Delete deletes all records with round less than the given round.
func (db *PostDb) Delete(table interface{}, round uint64) error {
	_, err := db.Db.NewDelete().Model(table).Where("round < ?", round).Exec(context.Background())
	return err
}

// GetBlockRound queries block round by block hash.
func (db *PostDb) GetBlockRound(hash string) (uint64, error) {
	block := new(model.BlockRef)
	err := db.Db.NewSelect().Model(block).Where("hash = ?", hash).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockHash queries block hash by block round.
func (db *PostDb) GetBlockHash(round uint64) (string, error) {
	block := new(model.BlockRef)
	err := db.Db.NewSelect().Model(block).Where("round = ?", round).Scan(context.Background())
	if err != nil {
		return "", err
	}

	return block.Hash, nil
}

// GetLatestBlockHash queries for the block hash of the latest round.
func (db *PostDb) GetLatestBlockHash() (string, error) {
	block := new(model.BlockRef)
	err := db.Db.NewSelect().Model(block).Order("round DESC").Limit(1).Scan(context.Background())
	if err != nil {
		return "", err
	}

	return block.Hash, nil
}

// GetContinuesIndexedRound queries latest continues indexed block round.
func (db *PostDb) GetContinuesIndexedRound() (uint64, error) {
	indexedRound := new(model.ContinuesIndexedRound)
	err := db.Db.NewSelect().Model(indexedRound).Where("tip = ?", model.Continues).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return indexedRound.Round, nil
}
