package psql

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-pg/pg/v10"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
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
		DialTimeout: time.Duration(cfg.PostDb.Timeout),
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

// GetBlockByHash queries block by block hash.
func (db *PostDb) GetBlockByHash(ctx context.Context, blockHash hash.Hash) (*block.Block, error) {
	return nil, nil
}

// GetBlockByRound queries block by round.
func (db *PostDb) GetBlockByRound(ctx context.Context, round uint64) (*block.Block, error) {
	return nil, nil
}

// GetBlockRoundByHash queries block round by block hash.
func (db *PostDb) GetBlockRoundByHash(ctx context.Context, blockHash hash.Hash) (uint64, error) {
	return 0, nil
}

// GetBlockHashByRound queries block hash by round.
func (db *PostDb) GetBlockHashByRound(ctx context.Context, round uint64) (hash.Hash, error) {
	return hash.Hash{}, nil
}

// GetTxByIndex queries tx by block round and index.
func (db *PostDb) GetTxByIndex(ctx context.Context, round uint64, index uint32) (hash.Hash, error) {
	return hash.Hash{}, nil
}

// StoreBlock stores block hash, round and block.
func (db *PostDb) StoreBlock(ctx context.Context, blockHash hash.Hash, round uint64, block *block.Block) error {
	return nil
}

// StoreTx stores tx hash, round and tx index.
func (db *PostDb) StoreTx(ctx context.Context, txHash hash.Hash, round uint64, index uint32) error {
	return nil
}
