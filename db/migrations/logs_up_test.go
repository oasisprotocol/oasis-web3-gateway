package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/oasisprotocol/oasis-web3-gateway/log"
	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

func InitTests(t *testing.T) *bun.DB {
	tests.MustInitConfig()

	// Init DB.
	dbCfg := tests.TestsConfig.Database
	pgConn := pgdriver.NewConnector(
		pgdriver.WithAddr(fmt.Sprintf("%v:%v", dbCfg.Host, dbCfg.Port)),
		pgdriver.WithDatabase(dbCfg.DB),
		pgdriver.WithUser(dbCfg.User),
		pgdriver.WithPassword(dbCfg.Password),
		pgdriver.WithTLSConfig(nil),
		pgdriver.WithDialTimeout(time.Duration(dbCfg.DialTimeout)*time.Second),
		pgdriver.WithReadTimeout(time.Duration(dbCfg.ReadTimeout)*time.Second),
		pgdriver.WithWriteTimeout(time.Duration(dbCfg.WriteTimeout)*time.Second))

	sqlDB := sql.OpenDB(pgConn)
	maxOpenConns := dbCfg.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = 4 * runtime.GOMAXPROCS(0)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	db := bun.NewDB(sqlDB, pgdialect.New())

	// Init logging.
	_ = log.InitLogging(tests.TestsConfig)

	return db
}

func TestLogsMigration(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	psql := InitTests(t)
	defer func() {
		_ = DropTables(ctx, psql)
	}()
	require.NoError(Init(ctx, psql), "initializing migrator")
	require.NoError(Migrate(ctx, psql), "running migrations")

	storage := Storage{DB: psql}

	// Prepare state.
	tx1 := &Transaction{
		Hash:      "world",
		Round:     1,
		Type:      1,
		ChainID:   "12321",
		Gas:       43568,
		GasPrice:  "437231",
		GasTipCap: "0",
		GasFeeCap: "0",
		Nonce:     2,
		ToAddr:    "worldworld",
		Value:     "2137000000000000000",
		Data:      "abcdefabcdef",
		V:         big.NewInt(2).String(),
		R:         big.NewInt(2).String(),
		S:         big.NewInt(2).String(),
	}
	tx2 := &Transaction{
		Hash:      "good",
		Round:     1,
		Type:      2,
		ChainID:   "45654",
		Gas:       2367215,
		GasPrice:  "0",
		GasTipCap: "123123",
		GasFeeCap: "345321",
		Nonce:     3,
		ToAddr:    "goodgood",
		Value:     "1123450000000000000",
		Data:      "123456123456",
		V:         big.NewInt(3).String(),
		R:         big.NewInt(3).String(),
		S:         big.NewInt(3).String(),
	}
	tx3 := &Transaction{
		Hash:      "good",
		Round:     2,
		Type:      2,
		ChainID:   "45654",
		Gas:       2367215,
		GasPrice:  "0",
		GasTipCap: "123123",
		GasFeeCap: "345321",
		Nonce:     3,
		ToAddr:    "goodgood",
		Value:     "1123450000000000000",
		Data:      "123456123456",
		V:         big.NewInt(3).String(),
		R:         big.NewInt(3).String(),
		S:         big.NewInt(3).String(),
	}
	// Invalid state before migration: logs have duplicate indices.
	log1 := &Log{
		Round:  1,
		Index:  0,
		TxHash: tx1.Hash,
	}
	log2 := &Log{
		Round:  1,
		Index:  1,
		TxHash: tx1.Hash,
	}
	log3 := &Log{
		Round:  1,
		Index:  2,
		TxHash: tx1.Hash,
	}
	log4 := &Log{
		Round:  1,
		Index:  0,
		TxHash: tx2.Hash,
	}
	log5 := &Log{
		Round:  1,
		Index:  1,
		TxHash: tx2.Hash,
	}
	log6 := &Log{
		Round:  2,
		Index:  0,
		TxHash: tx3.Hash,
	}
	log7 := &Log{
		Round:  2,
		Index:  11,
		TxHash: tx3.Hash,
	}
	startRound := &IndexedRoundWithTip{
		Tip:   LastRetained,
		Round: 1,
	}
	endRound := &IndexedRoundWithTip{
		Tip:   Continues,
		Round: 2,
	}

	require.NoError(storage.Upsert(ctx, tx1), "tx1 upsert")
	require.NoError(storage.Upsert(ctx, tx2), "tx2 upsert")
	require.NoError(storage.Upsert(ctx, log1), "log1 upsert")
	require.NoError(storage.Upsert(ctx, log2), "log2 upsert")
	require.NoError(storage.Upsert(ctx, log3), "log3 upsert")
	require.NoError(storage.Upsert(ctx, log4), "log4 upsert")
	require.NoError(storage.Upsert(ctx, log5), "log5 upsert")
	require.NoError(storage.Upsert(ctx, log6), "log6 upsert")
	require.NoError(storage.Upsert(ctx, log7), "log7 upsert")
	require.NoError(storage.Upsert(ctx, startRound), "start upsert")
	require.NoError(storage.Upsert(ctx, endRound), "end upsert")

	// Run migration.
	require.NoError(
		psql.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			return LogsUp(ctx, &tx)
		}),
		"logs up migration",
	)

	// Ensure logs updated.
	// Round 1.
	logs, err := storage.GetLogs(ctx, 1, 1)
	require.NoError(err, "GetLogs")
	for i, log := range logs {
		require.EqualValues(uint(i), log.Index, "expected log index after migration")
	}
	// Round 2.
	logs, err = storage.GetLogs(ctx, 2, 2)
	require.NoError(err, "GetLogs")
	for i, log := range logs {
		require.EqualValues(uint(i), log.Index, "expected log index after migration")
	}
}
