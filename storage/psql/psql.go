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
	"time"
)

type PostDb struct {
	Db *bun.DB
}

// InitDb creates postgresql db instance
func InitDb(cfg *conf.DatabaseConfig) (*PostDb, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}
	pgConn := pgdriver.NewConnector(
		pgdriver.WithAddr(fmt.Sprintf("%v:%v", cfg.Host, cfg.Port)),
		pgdriver.WithDatabase(cfg.Db),
		pgdriver.WithUser(cfg.User),
		pgdriver.WithPassword(cfg.Password),
		pgdriver.WithDialTimeout(time.Duration(cfg.DialTimeout)*time.Second),
		pgdriver.WithReadTimeout(time.Duration(cfg.ReadTimeout)*time.Second),
		pgdriver.WithWriteTimeout(time.Duration(cfg.WriteTimeout)*time.Second))
	// open
	sqlDB := sql.OpenDB(pgConn)
	// create db
	db := bun.NewDB(sqlDB, pgdialect.New())
	// create tables
	model.CreateTables(db)

	return &PostDb{Db: db}, nil
}

// GetTransactionRef returns block hash, round and index of the transaction.
func (db *PostDb) GetTransactionRef(txHash string) (*model.TransactionRef, error) {
	tx := new(model.TransactionRef)
	err := db.Db.NewSelect().Model(tx).Where("eth_tx_hash = ? ", txHash).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// GetTransaction returns ethereum transaction by hash.
func (db *PostDb) GetTransaction(hash string) (*model.Transaction, error) {
	tx := new(model.Transaction)
	err := db.Db.NewSelect().Model(tx).Where("hash = ?", hash).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// Exist returns whether the record exists.
func (db *PostDb) Exist(value interface{}) (bool, error) {
	count, err := db.Db.NewSelect().Model(value).WherePK().Count(context.Background())
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// upsert updates record when PK conflicts, otherwise inserts.
func (db *PostDb) upsert(value interface{}) error {
	// PKs are required for ON CONFLICT DO UPDATE
	//pks := db.Db.Model(value).Table().TableModel().Table().PKs
	//b := ""
	//for i, f := range pks {
	//	if i > 0 {
	//		b += ","
	//	}
	//	b += string(f.Column)
	//}
	//_, err := db.Db.Model(value).
	//	OnConflict(fmt.Sprintf("(%s) DO UPDATE", b)).
	//	Insert()

	exist, err := db.Exist(value)
	if err != nil {
		return err
	}
	if exist {
		_, err = db.Db.NewUpdate().Model(value).WherePK().Exec(context.Background())
	} else {
		_, err = db.Db.NewInsert().Model(value).Exec(context.Background())
	}

	return err
}

// Store stores data in db.
func (db *PostDb) Store(value interface{}) error {
	return db.upsert(value)
}

// Update updates record.
func (db *PostDb) Update(value interface{}) error {
	return db.upsert(value)
}

// Delete deletes all records with round less than the given round.
func (db *PostDb) Delete(table interface{}, round uint64) error {
	_, err := db.Db.NewDelete().Model(table).Where("round < ?", round).Exec(context.Background())
	return err
}

// GetBlockRound returns block round by block hash.
func (db *PostDb) GetBlockRound(hash string) (uint64, error) {
	block := new(model.BlockRef)
	err := db.Db.NewSelect().Model(block).Where("hash = ?", hash).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockHash returns block hash by block round.
func (db *PostDb) GetBlockHash(round uint64) (string, error) {
	block := new(model.BlockRef)
	err := db.Db.NewSelect().Model(block).Where("round = ?", round).Scan(context.Background())
	if err != nil {
		return "", err
	}

	return block.Hash, nil
}

// GetLatestBlockHash returns for the block hash of the latest round.
func (db *PostDb) GetLatestBlockHash() (string, error) {
	block := new(model.BlockRef)
	err := db.Db.NewSelect().Model(block).Order("round DESC").Limit(1).Scan(context.Background())
	if err != nil {
		return "", err
	}

	return block.Hash, nil
}

// GetContinuesIndexedRound returns latest continues indexed block round.
func (db *PostDb) GetContinuesIndexedRound() (uint64, error) {
	indexedRound := new(model.ContinuesIndexedRound)
	err := db.Db.NewSelect().Model(indexedRound).Where("tip = ?", model.Continues).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return indexedRound.Round, nil
}

// GetLastRetainedRound returns the minimum round not pruned.
func (db *PostDb) GetLastRetainedRound() (uint64, error) {
	retainedRound := new(model.IndexedRoundWithTip)
	err := db.Db.NewSelect().Model(retainedRound).Where("tip = ?", model.LastRetained).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return retainedRound.Round, nil
}

// GetLatestBlockNumber returns the latest block number.
func (db *PostDb) GetLatestBlockNumber() (uint64, error) {
	block := new(model.Block)
	err := db.Db.NewSelect().Model(block).Order("round DESC").Limit(1).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockByHash returns the block for the given hash.
func (db *PostDb) GetBlockByHash(blockHash string) (*model.Block, error) {
	blk := new(model.Block)
	err := db.Db.NewSelect().Model(blk).Where("hash = ?", blockHash).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockByNumber returns the block for the given round.
func (db *PostDb) GetBlockByNumber(round uint64) (*model.Block, error) {
	block := new(model.Block)
	err := db.Db.NewSelect().Model(block).Where("round = ?", round).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return block, nil
}

// GetBlockTransactionCountByNumber returns the count of transactions in block by block number.
func (db *PostDb) GetBlockTransactionCountByNumber(round uint64) (int, error) {
	block := new(model.Block)
	err := db.Db.NewSelect().Model(block).Where("round = ?", round).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return len(block.Transactions), nil
}

// GetBlockTransactionCountByHash returns the count of transactions in block by block hash.
func (db *PostDb) GetBlockTransactionCountByHash(blockHash string) (int, error) {
	block := new(model.Block)
	err := db.Db.NewSelect().Model(block).Where("hash = ?", blockHash).Scan(context.Background())
	if err != nil {
		return 0, err
	}

	return len(block.Transactions), nil
}

// GetBlockTransaction returns transaction by bock hash and transaction index.
func (db *PostDb) GetBlockTransaction(blockHash string, txIndex int) (*model.Transaction, error) {
	block := new(model.Block)
	err := db.Db.NewSelect().Model(block).Where("hash = ?", blockHash).Scan(context.Background())
	if err != nil {
		return nil, err
	}
	if len(block.Transactions) == 0 {
		return nil, errors.New("the block doesn't has any transactions")
	}
	if len(block.Transactions)-1 < txIndex {
		return nil, errors.New("index out of range")
	}

	return block.Transactions[txIndex], nil
}

// GetTransactionReceipt returns receipt by transaction hash.
func (db *PostDb) GetTransactionReceipt(txHash string) (*model.Receipt, error) {
	receipt := new(model.Receipt)
	err := db.Db.NewSelect().Model(receipt).Where("transaction_hash = ?", txHash).Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

// GetLogs return the logs by block hash and round.
func (db *PostDb) GetLogs(blockHash string, startRound, endRound uint64) ([]*model.Log, error) {
	logs := []*model.Log{}
	err := db.Db.NewSelect().Model(&logs).
		Where("block_hash = ? AND (round BETWEEN ? AND ?)", blockHash, startRound, endRound).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}

	return logs, nil
}
