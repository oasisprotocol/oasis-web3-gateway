package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/oasisprotocol/emerald-web3-gateway/conf"
	glog "github.com/oasisprotocol/emerald-web3-gateway/log"
	"github.com/oasisprotocol/emerald-web3-gateway/tests"
)

func Init(ctx context.Context, cfg *conf.DatabaseConfig) (*bun.DB, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}

	pgConn := pgdriver.NewConnector(
		pgdriver.WithAddr(fmt.Sprintf("%v:%v", cfg.Host, cfg.Port)),
		pgdriver.WithDatabase(cfg.DB),
		pgdriver.WithUser(cfg.User),
		pgdriver.WithPassword(cfg.Password),
		pgdriver.WithTLSConfig(nil),
		pgdriver.WithDialTimeout(time.Duration(cfg.DialTimeout)*time.Second),
		pgdriver.WithReadTimeout(time.Duration(cfg.ReadTimeout)*time.Second),
		pgdriver.WithWriteTimeout(time.Duration(cfg.WriteTimeout)*time.Second))

	sqlDB := sql.OpenDB(pgConn)
	maxOpenConns := cfg.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = 4 * runtime.GOMAXPROCS(0)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)

	// create db
	db := bun.NewDB(sqlDB, pgdialect.New())

	return db, nil
}

var storage *Storage

func TestMain(m *testing.M) {
	var err error
	ctx := context.Background()
	tests.MustInitConfig()
	psql, err := Init(ctx, tests.TestsConfig.Database)
	if err != nil {
		log.Println(`It seems database failed to initialize. Do you have PostgreSQL running? If not, you can run
docker run  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=postgres  -p 5432:5432 -d postgres`)
		log.Fatal("failed to initialize db:", err)
	}
	if err = glog.InitLogging(tests.TestsConfig); err != nil {
		log.Fatal("failed to initialize logging:", err)
	}
	if err = Migrate(ctx, psql); err != nil {
		log.Fatal("failed to run migrations:", err)
	}

	var code int
	if err = psql.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		storage = &Storage{DB: &tx}

		code = m.Run()

		return nil
	}); err != nil {
		log.Fatal("failed to run test: ", err)
	}

	if err = DropTables(ctx, psql); err != nil {
		log.Fatal("failed to cleanup db:", err)
	}

	os.Exit(code)
}

func TestLogsMigration(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

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
	require.NoError(LogsUp(ctx, storage.DB), "logs up migration")

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
