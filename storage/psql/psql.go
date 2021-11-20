package psql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-pg/pg/v10"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

type PostDb struct {
	Db *pg.DB
}

// InitDb creates postgresql db instance
func InitDb(cfg *conf.DatabaseConfig) (*PostDb, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}

	// Connect db
	db := pg.Connect(&pg.Options{
		Addr:        fmt.Sprintf("%v:%v", cfg.Host, cfg.Port),
		Database:    cfg.Db,
		User:        cfg.User,
		Password:    cfg.Password,
		DialTimeout: time.Duration(cfg.Timeout) * time.Second,
	})
	// Ping
	if err := db.Ping(context.Background()); err != nil {
		return nil, err
	}
	// initialize models
	if err := model.InitModel(db); err != nil {
		return nil, err
	}

	return &PostDb{
		Db: db,
	}, nil
}

// GetTransactionRef returns block hash, round and index of the transaction.
func (db *PostDb) GetTransactionRef(txHash string) (*model.TransactionRef, error) {
	tx := new(model.TransactionRef)
	err := db.Db.Model(tx).
		Where("eth_tx_hash=?", txHash).
		Select()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// GetTransaction returns ethereum transaction by hash.
func (db *PostDb) GetTransaction(hash string) (*model.Transaction, error) {
	tx := new(model.Transaction)
	err := db.Db.Model(tx).
		Where("hash=?", hash).
		Select()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// insertOrUpdate updates record when it exists, or inserts.
func (db *PostDb) insertOrUpdate(value interface{}) error {
	query := db.Db.Model(value).WherePK()
	exist, err := query.Exists()
	if err != nil {
		return err
	}

	if exist {
		_, err = query.Update()
	} else {
		_, err = query.Insert()
	}

	return err
}

// Store stores data in db.
func (db *PostDb) Store(value interface{}) error {
	return db.insertOrUpdate(value)
}

// Update updates record.
func (db *PostDb) Update(value interface{}) error {
	return db.insertOrUpdate(value)
}

// Delete deletes all records with round less than the given round.
func (db *PostDb) Delete(table interface{}, round uint64) error {
	_, err := db.Db.Model(table).Where("round<?", round).Delete()
	return err
}

// GetBlockRound returns block round by block hash.
func (db *PostDb) GetBlockRound(hash string) (uint64, error) {
	block := new(model.Block)
	err := db.Db.Model(block).
		Where("hash=?", hash).
		Select()
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockHash returns block hash by block round.
func (db *PostDb) GetBlockHash(round uint64) (string, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).
		Where("round=?", round).
		Select()
	if err != nil {
		return "", err
	}

	return blk.Hash, nil
}

// GetLatestBlockHash returns for the block hash of the latest round.
func (db *PostDb) GetLatestBlockHash() (string, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Order("round DESC").Limit(1).Select()
	if err != nil {
		return "", err
	}

	return blk.Hash, nil
}

// GetContinuesIndexedRound returns latest continues indexed block round.
func (db *PostDb) GetContinuesIndexedRound() (uint64, error) {
	indexedRound := new(model.IndexedRoundWithTip)
	err := db.Db.Model(indexedRound).
		Where("tip=?", model.Continues).
		Select()
	if err != nil {
		return 0, err
	}

	return indexedRound.Round, nil
}

// GetLastRetainedRound returns the minimum round not pruned.
func (db *PostDb) GetLastRetainedRound() (uint64, error) {
	retainedRound := new(model.IndexedRoundWithTip)
	err := db.Db.Model(retainedRound).
		Where("tip=?", model.LastRetained).
		Select()
	if err != nil {
		return 0, err
	}

	return retainedRound.Round, nil
}

// GetLatestBlockNumber returns the latest block number.
func (db *PostDb) GetLatestBlockNumber() (uint64, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Order("round DESC").Limit(1).Select()
	if err != nil {
		return 0, err
	}

	return blk.Round, nil
}

// GetBlockByHash returns the block for the given hash.
func (db *PostDb) GetBlockByHash(blockHash string) (*model.Block, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("hash=?", blockHash).Select()
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockByNumber returns the block for the given round.
func (db *PostDb) GetBlockByNumber(round uint64) (*model.Block, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("round=?", round).Select()
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockTransactionCountByNumber returns the count of transactions in block by block number.
func (db *PostDb) GetBlockTransactionCountByNumber(round uint64) (int, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("round=?", round).Select()
	if err != nil {
		return 0, err
	}

	return len(blk.Transactions), nil
}

// GetBlockTransactionCountByHash returns the count of transactions in block by block hash.
func (db *PostDb) GetBlockTransactionCountByHash(blockHash string) (int, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("hash=?", blockHash).Select()
	if err != nil {
		return 0, err
	}

	return len(blk.Transactions), nil
}

// GetBlockTransaction returns transaction by bock hash and transaction index.
func (db *PostDb) GetBlockTransaction(blockHash string, txIndex int) (*model.Transaction, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("hash=?", blockHash).Select()
	if err != nil {
		return nil, err
	}
	if len(blk.Transactions) == 0 {
		return nil, errors.New("the block doesn't has any transactions")
	}
	if len(blk.Transactions)-1 < txIndex {
		return nil, errors.New("out of index")
	}

	return blk.Transactions[txIndex], nil
}

// GetTransactionReceipt returns receipt by transaction hash.
func (db *PostDb) GetTransactionReceipt(txHash string) (*model.Receipt, error) {
	receipt := new(model.Receipt)
	if err := db.Db.Model(receipt).Where("transaction_hash=?", txHash).Select(); err != nil {
		return nil, err
	}

	return receipt, nil
}

// GetLogs return the logs by block hash and round.
func (db *PostDb) GetLogs(blockHash string, startRound, endRound uint64) ([]*model.Log, error) {
	logs := []*model.Log{}
	err := db.Db.Model(&logs).
		Where("block_hash=? AND (round BETWEEN ? AND ?)", blockHash, startRound, endRound).Select()
	if err != nil {
		return nil, err
	}

	return logs, nil
}
