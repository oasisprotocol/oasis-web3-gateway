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
func InitDb(cfg *conf.Config) (*PostDb, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}
	// Connect db
	db := pg.Connect(&pg.Options{
		Addr:        fmt.Sprintf("%v:%v", cfg.PostDb.Host, cfg.PostDb.Port),
		Database:    cfg.PostDb.Db,
		User:        cfg.PostDb.User,
		Password:    cfg.PostDb.Password,
		DialTimeout: time.Duration(cfg.PostDb.Timeout) * time.Second,
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

//GetTransactionRoundAndIndex queries transaction round and index by hash.
func (db *PostDb) GetTransactionRoundAndIndex(hash string) (uint64, uint32, error) {
	tx := new(model.TransactionRef)
	err := db.Db.Model(tx).
		Where("transaction_ref.eth_tx_hash=?", hash).
		Select()
	if err != nil {
		return 0, 0, err
	}

	return tx.Round, tx.Index, nil
}

// GetTransactionByRoundAndIndex queries ethereum transaction by round and index.
func (db *PostDb) GetTransactionByRoundAndIndex(round uint64, index uint32) (*model.Transaction, error) {
	txRef := new(model.TransactionRef)
	err := db.Db.Model(txRef).
		Where("transaction_ref.round=? and transaction_ref.index=?", round, index).
		Select()
	if err != nil {
		return nil, err
	}

	ethTx := new(model.Transaction)
	err = db.Db.Model(ethTx).
		Where("eth_transaction.hash=?", txRef.EthTxHash).
		Select()
	if err != nil {
		return nil, err
	}

	return ethTx, nil
}

// GetTransaction queries ethereum transaction by hash.
func (db *PostDb) GetTransaction(hash string) (*model.Transaction, error) {
	tx := new(model.Transaction)
	err := db.Db.Model(tx).
		Where("eth_transaction.hash=?", hash).
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

// GetBlockRound queries block round by block hash.
func (db *PostDb) GetBlockRound(hash string) (uint64, error) {
	block := new(model.BlockRef)
	err := db.Db.Model(block).
		Where("block_ref.hash=?", hash).
		Select()
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockHash queries block hash by block round.
func (db *PostDb) GetBlockHash(round uint64) (string, error) {
	blk := new(model.BlockRef)
	err := db.Db.Model(blk).
		Where("block_ref.round=?", round).
		Select()
	if err != nil {
		return "", err
	}

	return blk.Hash, nil
}

// GetContinuesIndexedRound queries latest continues indexed block round.
func (db *PostDb) GetContinuesIndexedRound() (uint64, error) {
	indexedRound := new(model.ContinuesIndexedRound)
	err := db.Db.Model(indexedRound).
		Where("continues_indexed_round.tip=?", model.Continues).
		Select()
	if err != nil {
		return 0, err
	}

	return indexedRound.Round, nil
}
