package psql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/roothash/api/block"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
)

type PostDb struct {
	Db *sql.DB
}

// InitDb creates postdb instance
func InitDb(cfg *conf.Config) (*PostDb, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}
	conn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		cfg.PostDb.Host, cfg.PostDb.Port, cfg.PostDb.User, cfg.PostDb.Password, cfg.PostDb.Db, cfg.PostDb.SslMode, cfg.PostDb.Timeout)

	db, err := sql.Open("postgres", conn)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
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
