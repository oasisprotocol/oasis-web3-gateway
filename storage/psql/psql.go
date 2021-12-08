package psql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-pg/pg/v10"

	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

type PostDB struct {
	DB pg.DBI
}

// InitDb creates postgresql db instance.
func InitDB(cfg *conf.DatabaseConfig) (*PostDB, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}

	// Connect db.
	db := pg.Connect(&pg.Options{
		Addr:        fmt.Sprintf("%v:%v", cfg.Host, cfg.Port),
		Database:    cfg.DB,
		User:        cfg.User,
		Password:    cfg.Password,
		DialTimeout: time.Duration(cfg.Timeout) * time.Second,
	})
	// Ping.
	if err := db.Ping(context.Background()); err != nil {
		return nil, err
	}
	// Initialize models.
	if err := model.InitModel(db); err != nil {
		return nil, err
	}

	return &PostDB{
		DB: db,
	}, nil
}

// GetTransactionRef returns block hash, round and index of the transaction.
func (db *PostDB) GetTransactionRef(txHash string) (*model.TransactionRef, error) {
	tx := new(model.TransactionRef)
	err := db.DB.Model(tx).
		Where("eth_tx_hash=?", txHash).
		Select()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// GetTransaction queries ethereum transaction by hash.
func (db *PostDB) GetTransaction(hash string) (*model.Transaction, error) {
	tx := new(model.Transaction)
	err := db.DB.Model(tx).
		Where("hash=?", hash).
		Select()
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// upsert updates record when PK conflicts, otherwise inserts.
func (db *PostDB) upsert(value interface{}) (err error) {
	switch values := value.(type) {
	case []interface{}:
		for v := range values {
			err = db.upsertSingle(v)
		}
	case interface{}:
		err = db.upsertSingle(value)
	}

	return
}

// upsertSingle updates record when PK conflicts, otherwise inserts.
// value must be a non-array interface.
func (db *PostDB) upsertSingle(value interface{}) error {
	query := db.DB.Model(value).WherePK()
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
func (db *PostDB) Upsert(value interface{}) error {
	return db.upsert(value)
}

// Delete deletes all records with round less than the given round.
func (db *PostDB) Delete(table interface{}, round uint64) error {
	_, err := db.DB.Model(table).Where("round<?", round).Delete()
	return err
}

// GetBlockRound returns block round by block hash.
func (db *PostDB) GetBlockRound(hash string) (uint64, error) {
	block := new(model.Block)
	err := db.DB.Model(block).
		Where("hash=?", hash).
		Select()
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockHash returns block hash by block round.
func (db *PostDB) GetBlockHash(round uint64) (string, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).
		Where("round=?", round).
		Select()
	if err != nil {
		return "", err
	}

	return blk.Hash, nil
}

// GetLatestBlockHash returns for the block hash of the latest round.
func (db *PostDB) GetLatestBlockHash() (string, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Order("round DESC").Limit(1).Select()
	if err != nil {
		return "", err
	}

	return blk.Hash, nil
}

// GetContinuesIndexedRound returns latest continues indexed block round.
func (db *PostDB) GetContinuesIndexedRound() (uint64, error) {
	indexedRound := new(model.IndexedRoundWithTip)
	err := db.DB.Model(indexedRound).
		Where("tip=?", model.Continues).
		Select()
	if err != nil {
		if errors.Is(err, pg.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return indexedRound.Round, nil
}

// GetLastRetainedRound returns the minimum round not pruned.
func (db *PostDB) GetLastRetainedRound() (uint64, error) {
	retainedRound := new(model.IndexedRoundWithTip)
	err := db.DB.Model(retainedRound).
		Where("tip=?", model.LastRetained).
		Select()
	if err != nil {
		if errors.Is(err, pg.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return retainedRound.Round, nil
}

// GetLatestBlockNumber returns the latest block number.
func (db *PostDB) GetLatestBlockNumber() (uint64, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Order("round DESC").Limit(1).Select()
	if err != nil {
		return 0, err
	}

	return blk.Round, nil
}

// GetBlockByHash returns the block for the given hash.
func (db *PostDB) GetBlockByHash(blockHash string) (*model.Block, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Where("hash=?", blockHash).Relation("Transactions").Select()
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockByNumber returns the block for the given round.
func (db *PostDB) GetBlockByNumber(round uint64) (*model.Block, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Where("round=?", round).Relation("Transactions").Select()
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockTransactionCountByNumber returns the count of transactions in block by block number.
func (db *PostDB) GetBlockTransactionCountByNumber(round uint64) (int, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Where("round=?", round).Relation("Transactions").Select()
	if err != nil {
		return 0, err
	}

	return len(blk.Transactions), nil
}

// GetBlockTransactionCountByHash returns the count of transactions in block by block hash.
func (db *PostDB) GetBlockTransactionCountByHash(blockHash string) (int, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Where("hash=?", blockHash).Relation("Transactions").Select()
	if err != nil {
		return 0, err
	}

	return len(blk.Transactions), nil
}

// GetBlockTransaction returns transaction by bock hash and transaction index.
func (db *PostDB) GetBlockTransaction(blockHash string, txIndex int) (*model.Transaction, error) {
	blk := new(model.Block)
	err := db.DB.Model(blk).Where("hash=?", blockHash).Relation("Transactions").Select()
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
func (db *PostDB) GetTransactionReceipt(txHash string) (*model.Receipt, error) {
	receipt := new(model.Receipt)
	if err := db.DB.Model(receipt).Where("transaction_hash=?", txHash).Relation("Logs").Select(); err != nil {
		return nil, err
	}

	return receipt, nil
}

// GetLogs return the logs by block hash and round.
func (db *PostDB) GetLogs(startRound, endRound uint64) ([]*model.Log, error) {
	logs := []*model.Log{}
	err := db.DB.Model(&logs).
		Where("(round BETWEEN ? AND ?)", startRound, endRound).Select()
	if err != nil {
		return nil, err
	}

	return logs, nil
}

func transactionStorage(t *pg.Tx) storage.Storage {
	db := PostDB{t}
	return &db
}

// RunInTransaction runs a function in a transaction. If function
// returns an error transaction is rolled back, otherwise transaction
// is committed.
func (db *PostDB) RunInTransaction(ctx context.Context, fn func(storage.Storage) error) error {
	return db.DB.RunInTransaction(ctx, func(t *pg.Tx) error {
		db := transactionStorage(t)
		return fn(db)
	})
}
