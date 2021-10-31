package psql

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"time"

	"github.com/go-pg/pg/v10"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
)

type PostDb struct {
	Db *pg.DB
}

// InitDb creates postgresql db instance
func InitDb(cfg *conf.PostDbConfig) (*PostDb, error) {
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
	if err := db.Ping(context.TODO()); err != nil {
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

// Store stores data.
func (db *PostDb) Store(value interface{}) error {
	_, err := db.Db.Model(value).Insert()
	return err
}

// Update updates record.
func (db *PostDb) Update(value interface{}) error {
	var err error
	query := db.Db.Model(value)
	exists, _ := query.WherePK().Exists()
	if !exists {
		_, err = query.Insert()
	} else {
		_, err = query.WherePK().Update()
	}
	return err
}

// Delete deletes all records with round less than the given round.
func (db *PostDb) Delete(table interface{}, round uint64) error {
	_, err := db.Db.Model(table).Where("round<?", round).Delete()
	return err
}

// GetBlockRound returns block round by block hash.
func (db *PostDb) GetBlockRound(hash string) (uint64, error) {
	block := new(model.BlockRef)
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
	blk := new(model.BlockRef)
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
	indexedRound := new(model.ContinuesIndexedRound)
	err := db.Db.Model(indexedRound).
		Where("tip=?", model.Continues).
		Select()
	if err != nil {
		return 0, err
	}

	return indexedRound.Round, nil
}

func (db *PostDb) GetBlockNumber() (uint64, error) {
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

func (db *PostDb) GetBlockTransactionCountByNumber(round uint64) (int, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("round=?", round).Select()
	if err != nil {
		return 0, err
	}

	return len(blk.Transactions), nil
}

func (db *PostDb) GetBlockTransactionCountByHash(blockHash string) (int, error) {
	blk := new(model.Block)
	err := db.Db.Model(blk).Where("hash=?", blockHash).Select()
	if err != nil {
		return 0, err
	}

	return len(blk.Transactions), nil
}

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

func (db *PostDb) GetTransactionReceipt(txHash string) (map[string]interface{}, error) {
	tx := new(model.Transaction)
	if err := db.Db.Model(tx).Where("hash=?", txHash).Select(); err != nil {
		return nil, err
	}
	block := new(model.Block)
	if err := db.Db.Model(block).Where("hash=?", tx.BlockHash).Select(); err != nil {
		return nil, err
	}
	cumulativeGasUsed := uint64(0)
	for _, tx := range block.Transactions {
		cumulativeGasUsed += tx.Gas
	}
	receipt := map[string]interface{}{
		"status":            hexutil.Uint(0),
		"cumulativeGasUsed": hexutil.Uint64(cumulativeGasUsed),
		"logsBloom":         "",
		"logs":              "",
		"transactionHash":   tx.Hash,
		"gasUsed":           hexutil.Uint64(tx.Gas),
		"type":              hexutil.Uint64(tx.Type),
		"blockHash":         tx.BlockHash,
		"blockNumber":       hexutil.Uint64(tx.Round),
		"transactionIndex":  hexutil.Uint64(tx.Index),
		"from":              tx.FromAddr,
		"to":                tx.ToAddr,
	}

	return receipt, nil
}
