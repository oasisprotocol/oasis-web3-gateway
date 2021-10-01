package psql

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-pg/pg/v10"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"time"
)

type PostDb struct {
	Db *pg.DB
}

// InitDb creates postdb instance
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

// Store stores data.
func (db *PostDb) Store(value interface{}) error {
	_, err := db.Db.Model(value).Insert()
	return err
}

// GetBlockRound queries block round by block hash.
func (db *PostDb) GetBlockRound(hash string) (uint64, error) {
	blockRef := new(model.BlockRef)
	err := db.Db.Model(blockRef).
		Where("blockRef.hash=?", hash).
		Select()
	if err != nil {
		return 0, err
	}

	return blockRef.Round, nil
}

// GetBlockHash queries block hash by block round.
func (db *PostDb) GetBlockHash(round uint64) (string, error) {
	blk := new(model.BlockRef)
	err := db.Db.Model(blk).
		Where("blockRef.round=?", round).
		Select()
	if err != nil {
		return "", err
	}

	return blk.Hash, nil
}

// GetTxResult queries oasis tx result by ethereum tx hash.
func (db *PostDb) GetTxResult(hash string) (*model.TxResult, error) {
	tx := new(model.TransactionRef)
	err := db.Db.Model(tx).
		Where("TransactionRef.EthTxHash=?", hash).
		Select()
	if err != nil {
		return nil, err
	}

	return tx.Result, nil
}
