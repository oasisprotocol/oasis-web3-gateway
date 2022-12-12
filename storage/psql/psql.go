package psql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/migrations"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

// Long timeout used when database is opened with long timeouts option enabled.
// XXX: the driver does not support unlimited timeouts, so set long enough that
// should cover all reasonable future cases.
var longReadWriteTimeout = time.Hour * 12

type PostDB struct {
	DB bun.IDB
}

// InitDB creates postgresql db instance.
func InitDB(
	ctx context.Context,
	cfg *conf.DatabaseConfig,
	longTimeouts bool,
	readOnly bool,
) (*PostDB, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}

	readTimeout := time.Duration(cfg.ReadTimeout) * time.Second
	writeTimeout := time.Duration(cfg.WriteTimeout) * time.Second
	opts := []pgdriver.Option{
		pgdriver.WithAddr(fmt.Sprintf("%v:%v", cfg.Host, cfg.Port)),
		pgdriver.WithDatabase(cfg.DB),
		pgdriver.WithUser(cfg.User),
		pgdriver.WithPassword(cfg.Password),
		pgdriver.WithTLSConfig(nil),
		pgdriver.WithDialTimeout(time.Duration(cfg.DialTimeout) * time.Second),
	}

	// Set read/write timeouts.
	switch longTimeouts {
	case false:
		opts = append(opts,
			pgdriver.WithReadTimeout(readTimeout),
			pgdriver.WithWriteTimeout(writeTimeout),
		)
	case true:
		// Increase write/read timeouts if DB is being opened for migrations/manual actions.
		if longReadWriteTimeout > readTimeout {
			opts = append(opts, pgdriver.WithReadTimeout(longReadWriteTimeout))
		}
		if longReadWriteTimeout > writeTimeout {
			opts = append(opts, pgdriver.WithWriteTimeout(longReadWriteTimeout))
		}
	}

	// Set "read-only" mode by setting the default status of new
	// transactions.
	//
	// Note: This still allows txes to alter temporary tables, and is
	// advisory rather than something that is securely enforced.
	if readOnly {
		opts = append(opts, pgdriver.WithConnParams(
			map[string]interface{}{
				"default_transaction_read_only": "on",
			},
		))
	}

	pgConn := pgdriver.NewConnector(opts...)
	sqlDB := sql.OpenDB(pgConn)
	maxOpenConns := cfg.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = 4 * runtime.GOMAXPROCS(0)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)

	// create db
	db := bun.NewDB(sqlDB, pgdialect.New())

	return &PostDB{DB: db}, nil
}

func (db *PostDB) RunMigrations(ctx context.Context) error {
	// Initialize migrator.
	if err := migrations.Init(ctx, db.DB.(*bun.DB)); err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	// Run migrations.
	if err := migrations.Migrate(ctx, db.DB.(*bun.DB)); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	return nil
}

// GetTransaction queries ethereum transaction by hash.
func (db *PostDB) GetTransaction(ctx context.Context, hash string) (*model.Transaction, error) {
	tx := new(model.Transaction)
	err := db.DB.NewSelect().Model(tx).Where("hash = ?", hash).Scan(ctx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// Inserts a single record in the DB.
func (db *PostDB) Insert(ctx context.Context, value interface{}) error {
	switch values := value.(type) {
	case []interface{}:
		for v := range values {
			if _, err := db.DB.NewInsert().
				Model(v).
				Exec(ctx); err != nil {
				return err
			}
		}
	case interface{}:
		_, err := db.DB.NewInsert().
			Model(value).
			Exec(ctx)
		return err
	}
	return nil
}

func (db *PostDB) insertSingle(ctx context.Context, value interface{}, upsert bool) error {
	typ := reflect.TypeOf(value)
	table := db.DB.Dialect().Tables().Get(typ)
	pks := make([]string, len(table.PKs))
	for i, f := range table.PKs {
		pks[i] = f.Name
	}

	var onConflictAction string
	switch upsert {
	case true:
		onConflictAction = "UPDATE"
	case false:
		onConflictAction = "NOTHING"
	}

	_, err := db.DB.NewInsert().
		Model(value).
		On(fmt.Sprintf("CONFLICT (%s) DO %s", strings.Join(pks, ","), onConflictAction)).
		Exec(ctx)

	return err
}

// InsertIfNotExists inserts the record if it does not yet exist.
func (db *PostDB) InsertIfNotExists(ctx context.Context, value interface{}) error {
	switch values := value.(type) {
	case []interface{}:
		for v := range values {
			if err := db.insertSingle(ctx, v, false); err != nil {
				return err
			}
		}
	case interface{}:
		return db.insertSingle(ctx, value, false)
	}
	return nil
}

// Upsert upserts the record.
func (db *PostDB) Upsert(ctx context.Context, value interface{}) error {
	switch values := value.(type) {
	case []interface{}:
		for v := range values {
			if err := db.insertSingle(ctx, v, true); err != nil {
				return err
			}
		}
	case interface{}:
		return db.insertSingle(ctx, value, true)
	}
	return nil
}

// Delete deletes all records with round less than the given round.
func (db *PostDB) Delete(ctx context.Context, table interface{}, round uint64) error {
	_, err := db.DB.NewDelete().Model(table).Where("round < ?", round).Exec(ctx)
	return err
}

// GetBlockRound returns block round by block hash.
func (db *PostDB) GetBlockRound(ctx context.Context, hash string) (uint64, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Where("hash = ?", hash).Scan(ctx)
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockHash returns block hash by block round.
func (db *PostDB) GetBlockHash(ctx context.Context, round uint64) (string, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Where("round = ?", round).Scan(ctx)
	if err != nil {
		return "", err
	}

	return block.Hash, nil
}

// GetLatestBlockHash returns for the block hash of the latest round.
func (db *PostDB) GetLatestBlockHash(ctx context.Context) (string, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Order("round DESC").Limit(1).Scan(ctx)
	if err != nil {
		return "", err
	}

	return block.Hash, nil
}

// GetLastIndexedRound returns latest indexed block round.
func (db *PostDB) GetLastIndexedRound(ctx context.Context) (uint64, error) {
	indexedRound := new(model.IndexedRoundWithTip)
	err := db.DB.NewSelect().Model(indexedRound).Where("tip = ?", model.Continues).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, storage.ErrNoRoundsIndexed
		}
		return 0, err
	}

	return indexedRound.Round, nil
}

// GetLastRetainedRound returns the minimum round not pruned.
func (db *PostDB) GetLastRetainedRound(ctx context.Context) (uint64, error) {
	retainedRound := new(model.IndexedRoundWithTip)
	err := db.DB.NewSelect().Model(retainedRound).Where("tip = ?", model.LastRetained).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return retainedRound.Round, nil
}

// GetLatestBlockNumber returns the latest block number.
func (db *PostDB) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Order("round DESC").Limit(1).Scan(ctx)
	if err != nil {
		return 0, err
	}

	return block.Round, nil
}

// GetBlockByHash returns the block for the given hash.
func (db *PostDB) GetBlockByHash(ctx context.Context, blockHash string) (*model.Block, error) {
	blk := new(model.Block)
	err := db.DB.NewSelect().Model(blk).Where("hash = ?", blockHash).Relation("Transactions").Scan(ctx)
	if err != nil {
		return nil, err
	}

	return blk, nil
}

// GetBlockByNumber returns the block for the given round.
func (db *PostDB) GetBlockByNumber(ctx context.Context, round uint64) (*model.Block, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Where("round = ?", round).Relation("Transactions").Scan(ctx)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// GetBlockTransactionCountByNumber returns the count of transactions in block by block number.
func (db *PostDB) GetBlockTransactionCountByNumber(ctx context.Context, round uint64) (int, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Where("round = ?", round).Relation("Transactions").Scan(ctx)
	if err != nil {
		return 0, err
	}

	return len(block.Transactions), nil
}

// GetBlockTransactionCountByHash returns the count of transactions in block by block hash.
func (db *PostDB) GetBlockTransactionCountByHash(ctx context.Context, blockHash string) (int, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Where("hash = ?", blockHash).Relation("Transactions").Scan(ctx)
	if err != nil {
		return 0, err
	}

	return len(block.Transactions), nil
}

// GetBlockTransaction returns transaction by bock hash and transaction index.
func (db *PostDB) GetBlockTransaction(ctx context.Context, blockHash string, txIndex int) (*model.Transaction, error) {
	block := new(model.Block)
	err := db.DB.NewSelect().Model(block).Where("hash = ?", blockHash).Relation("Transactions").Scan(ctx)
	if err != nil {
		return nil, err
	}
	if len(block.Transactions) == 0 {
		return nil, errors.New("the block doesn't have any transactions")
	}
	if len(block.Transactions)-1 < txIndex {
		return nil, errors.New("index out of range")
	}

	return block.Transactions[txIndex], nil
}

// GetTransactionReceipt returns receipt by transaction hash.
func (db *PostDB) GetTransactionReceipt(ctx context.Context, txHash string) (*model.Receipt, error) {
	receipt := new(model.Receipt)
	err := db.DB.NewSelect().Model(receipt).Where("transaction_hash = ?", txHash).Relation("Logs").Scan(ctx)
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

// GetLogs return the logs by block hash and round.
func (db *PostDB) GetLogs(ctx context.Context, startRound, endRound uint64) ([]*model.Log, error) {
	logs := []*model.Log{}
	err := db.DB.NewSelect().Model(&logs).
		Where("round BETWEEN ? AND ?", startRound, endRound).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return logs, nil
}

func transactionStorage(t *bun.Tx) storage.Storage {
	db := PostDB{t}
	return &db
}

// RunInTransaction runs a function in a transaction. If function
// returns an error transaction is rolled back, otherwise transaction
// is committed.
func (db *PostDB) RunInTransaction(ctx context.Context, fn func(storage.Storage) error) error {
	bdb, ok := db.DB.(*bun.DB)
	if !ok {
		return fmt.Errorf("already in a transaction")
	}
	return bdb.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := transactionStorage(&tx)
		return fn(db)
	})
}
